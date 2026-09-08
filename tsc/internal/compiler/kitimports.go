package compiler

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
	"github.com/microsoft/TypeScript/tsc/internal/vfs"
)

type kitSymbolInfo struct {
	Source   string `json:"source"`
	Bindings string `json:"bindings"`
}

type kitJSONInfo struct {
	Symbols map[string]kitSymbolInfo `json:"symbols"`
}

type cachedKitJSON struct {
	info  kitJSONInfo
	found bool
}

// KitImportProcessor owns the per-compiler-host cache used by OH
// ohApi.ts::processKit. The transformed nodes exist only in TSGO's semantic
// program; Arkdown continues to use OXC/Rolldown for production code output.
type KitImportProcessor struct {
	mu    sync.Mutex
	cache map[string]cachedKitJSON
}

var kitErrorSymbolWhiteList = map[string]map[string]struct{}{
	"@kit.CoreFileKit": {"DfsListeners": {}},
	"@kit.NetworkKit":  {"VpnExtensionContext": {}},
	"@kit.ArkUI":       {"CustomContentDialog": {}},
}

var kitTSWarningWhiteList = map[string]map[string]struct{}{
	"@kit.ConnectivityKit": {"socket": {}},
}

var kitTSFileWhiteList = map[string]struct{}{
	"@kit.AccountKit":              {},
	"@kit.MapKit":                  {},
	"@kit.Penkit":                  {},
	"@kit.ScenarioFusionKit":       {},
	"@kit.ServiceCollaborationKit": {},
	"@kit.SpeechKit":               {},
	"@kit.VisionKit":               {},
	"@kit.PDFKit":                  {},
	"@kit.ReaderKit":               {},
}

var extendComponentWhiteList = map[string]struct{}{
	"ArcList": {}, "ArcListItem": {}, "MovingPhotoView": {}, "ArcSwiper": {},
	"ArcScrollBar": {}, "ArcAlphabetIndexer": {}, "HdsNavigation": {},
	"HdsNavDestination": {}, "HdsTabs": {}, "DotMatrix": {}, "Metaball": {},
	"HdsListItemCard": {}, "HdsVisualComponent": {}, "MultiWindowEntryInAPP": {},
	"AudioWave": {},
}

var extendComponentAttributeWhiteList = map[string]struct{}{
	"ArcListAttribute": {}, "ArcListItemAttribute": {}, "MovingPhotoViewAttribute": {},
	"ArcSwiperAttribute": {}, "ArcScrollBarAttribute": {}, "ArcAlphabetIndexerAttribute": {},
	"HdsNavigationAttribute": {}, "HdsNavDestinationAttribute": {}, "HdsTabsAttribute": {},
	"DotMatrixAttribute": {}, "MetaballAttribute": {}, "HdsListItemCardAttribute": {},
	"HdsVisualComponentAttribute": {}, "MultiWindowEntryInAPPAttribute": {},
	"AudioWaveAttribute": {},
}

var apiModuleWhiteList = map[string]struct{}{
	"@ohos.arkui.ArcList": {}, "@ohos.arkui.ArcSwiper": {},
	"@ohos.arkui.ArcScrollBar": {}, "@ohos.arkui.ArcAlphabetIndexer": {},
	"@ohos.multimedia.movingphotoview": {}, "@hms.hds.hdsBaseComponent": {},
	"@hms.hds.HdsVisualComponent": {}, "@hms.hds.dotMatrix": {},
	"@hms.hds.Metaball": {}, "@hms.hds.AudioWave": {},
}

var kitModuleWhiteList = map[string]struct{}{
	"@kit.ArkUI": {}, "@kit.MediaLibraryKit": {}, "@kit.UIDesignKit": {},
}

type orderedStringSet struct {
	values []string
	seen   map[string]struct{}
}

func (s *orderedStringSet) add(value string) {
	if s.seen == nil {
		s.seen = make(map[string]struct{})
	}
	if _, exists := s.seen[value]; exists {
		return
	}
	s.seen[value] = struct{}{}
	s.values = append(s.values, value)
}

// Process applies the source-owned ArkTS 1.1 @kit semantic expansion before
// binding. It returns true only when the source-file statement list changed.
func (p *KitImportProcessor) Process(file *ast.SourceFile, fs vfs.FS) bool {
	opts := file.ParseOptions()
	if opts.NoTransformedKitInParser || opts.EtsLoaderPath == "" || len(file.Diagnostics()) != 0 {
		return false
	}

	inEtsContext := file.ScriptKind == core.ScriptKindETS
	apiMap := make(map[string]*orderedStringSet)
	kitMap := make(map[string]*orderedStringSet)
	existingAttributes := make(map[string]struct{})
	p.preProcessSpecifiedImports(file.Statements.Nodes, apiMap, kitMap, existingAttributes)

	factory := ast.NewNodeFactory(ast.NodeFactoryHooks{})
	statements := make([]*ast.Node, 0, len(file.Statements.Nodes))
	markedRanges := make([]core.TextRange, 0)
	skipRestStatements := false
	changed := false
	for _, statement := range file.Statements.Nodes {
		if !skipRestStatements && inEtsContext && !ast.IsImportDeclaration(statement) {
			skipRestStatements = true
		}

		if !skipRestStatements && p.excludeForAPINamedBinding(statement, file.FileName()) {
			updated := p.processAPINamedBindings(factory, statement.AsImportDeclaration(), apiMap, existingAttributes)
			statements = append(statements, updated)
			changed = changed || updated != statement
			continue
		}

		if skipRestStatements || p.excludeForKitImport(statement) {
			statements = append(statements, statement)
			continue
		}

		declaration := statement.AsImportDeclaration()
		moduleName := declaration.ModuleSpecifier.Text()
		if !inEtsContext {
			if _, excluded := kitTSFileWhiteList[moduleName]; excluded {
				statements = append(statements, statement)
				continue
			}
		}

		info, found := p.getKitJSON(moduleName, opts.EtsLoaderPath, fs)
		newImports, ok := p.processKitDeclaration(factory, declaration, info, found, inEtsContext, kitMap, existingAttributes)
		if !ok {
			statements = append(statements, statement)
			continue
		}
		statements = append(statements, newImports...)
		markedRanges = append(markedRanges, statement.Loc)
		changed = true
	}

	if !changed {
		return false
	}
	list := factory.NewNodeList(statements)
	list.Loc = file.Statements.Loc
	file.Statements = list
	file.MarkedKitImportRanges = markedRanges
	ast.SetParentInChildren(file.AsNode())
	ast.SetExternalModuleIndicator(file, opts.ExternalModuleIndicatorOptions)
	parser.RecollectExternalModuleReferences(file)
	return true
}

func (p *KitImportProcessor) getKitJSON(name string, loaderPath string, fs vfs.FS) (kitJSONInfo, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cache != nil {
		if cached, exists := p.cache[name]; exists {
			return cached.info, cached.found
		}
	} else {
		p.cache = make(map[string]cachedKitJSON)
	}

	mixed := strings.HasSuffix(tspath.NormalizePath(loaderPath), "dynamic/build-tools/ets-loader")
	sdkPath := tspath.ResolvePath(loaderPath, "../../../..")
	ohosConfigRoot := "openharmony/ets/build-tools/ets-loader/kit_configs"
	hmsConfigRoot := "hms/ets/build-tools/ets-loader/kit_configs"
	if mixed {
		sdkPath = tspath.ResolvePath(loaderPath, "../../../../..")
		ohosConfigRoot = "openharmony/ets/dynamic/build-tools/ets-loader/kit_configs"
		hmsConfigRoot = "hms/ets/dynamic/build-tools/ets-loader/kit_configs"
	}

	var text string
	var ok bool
	for _, root := range []string{ohosConfigRoot, hmsConfigRoot} {
		path := tspath.ResolvePath(sdkPath, root, name+".json")
		if text, ok = fs.ReadFile(path); ok {
			break
		}
	}
	if !ok || text == "" {
		p.cache[name] = cachedKitJSON{}
		return kitJSONInfo{}, false
	}
	var info kitJSONInfo
	if err := json.Unmarshal([]byte(text), &info); err != nil {
		panic(fmt.Errorf("invalid OpenHarmony kit config %s: %w", name, err))
	}
	p.cache[name] = cachedKitJSON{info: info, found: true}
	return info, true
}

func (p *KitImportProcessor) preProcessSpecifiedImports(
	statements []*ast.Node,
	apiMap map[string]*orderedStringSet,
	kitMap map[string]*orderedStringSet,
	existingAttributes map[string]struct{},
) {
	for _, statement := range statements {
		if !p.isValidNamedImport(statement) {
			continue
		}
		moduleName := statement.AsImportDeclaration().ModuleSpecifier.Text()
		if _, ok := apiModuleWhiteList[moduleName]; ok {
			p.processSpecifiedImport(statement.AsImportDeclaration(), apiMap, moduleName, existingAttributes)
		} else if _, ok := kitModuleWhiteList[moduleName]; ok {
			p.processSpecifiedImport(statement.AsImportDeclaration(), kitMap, moduleName, existingAttributes)
		}
	}
}

func (p *KitImportProcessor) processSpecifiedImport(
	declaration *ast.ImportDeclaration,
	destination map[string]*orderedStringSet,
	moduleName string,
	existingAttributes map[string]struct{},
) {
	for _, element := range declaration.ImportClause.AsImportClause().NamedBindings.AsNamedImports().Elements.Nodes {
		name := element.Name().Text()
		if _, ok := extendComponentWhiteList[name]; ok {
			set := destination[moduleName]
			if set == nil {
				set = &orderedStringSet{}
				destination[moduleName] = set
			}
			set.add(name)
		} else if _, ok := extendComponentAttributeWhiteList[name]; ok {
			existingAttributes[name] = struct{}{}
		}
	}
}

func (p *KitImportProcessor) isValidNamedImport(statement *ast.Node) bool {
	if !ast.IsImportDeclaration(statement) {
		return false
	}
	declaration := statement.AsImportDeclaration()
	clause := declaration.ImportClause
	return clause != nil && clause.AsImportClause().NamedBindings != nil &&
		ast.IsNamedImports(clause.AsImportClause().NamedBindings) && ast.IsStringLiteral(declaration.ModuleSpecifier)
}

func (p *KitImportProcessor) excludeForAPINamedBinding(statement *ast.Node, fileName string) bool {
	if strings.HasSuffix(fileName, tspath.ExtensionDts) || !p.isValidNamedImport(statement) {
		return false
	}
	_, ok := apiModuleWhiteList[statement.AsImportDeclaration().ModuleSpecifier.Text()]
	return ok
}

func (p *KitImportProcessor) excludeForKitImport(statement *ast.Node) bool {
	if !ast.IsImportDeclaration(statement) {
		return true
	}
	declaration := statement.AsImportDeclaration()
	if declaration.ImportClause == nil || declaration.Modifiers() != nil || declaration.Attributes != nil ||
		!ast.IsStringLiteral(declaration.ModuleSpecifier) || !strings.HasPrefix(declaration.ModuleSpecifier.Text(), "@kit.") {
		return true
	}
	named := declaration.ImportClause.AsImportClause().NamedBindings
	return named != nil && ast.IsNamespaceImport(named)
}

func (p *KitImportProcessor) processAPINamedBindings(
	factory *ast.NodeFactory,
	declaration *ast.ImportDeclaration,
	apiMap map[string]*orderedStringSet,
	existingAttributes map[string]struct{},
) *ast.Node {
	moduleName := declaration.ModuleSpecifier.Text()
	set := apiMap[moduleName]
	if set == nil {
		return declaration.AsNode()
	}
	delete(apiMap, moduleName)
	return p.supplementNamedBindings(factory, declaration, set, existingAttributes)
}

func (p *KitImportProcessor) supplementNamedBindings(
	factory *ast.NodeFactory,
	declaration *ast.ImportDeclaration,
	components *orderedStringSet,
	existingAttributes map[string]struct{},
) *ast.Node {
	elements := declaration.ImportClause.AsImportClause().NamedBindings.AsNamedImports().Elements.Nodes
	updated := append([]*ast.Node(nil), elements...)
	for _, component := range components.values {
		attribute := component + "Attribute"
		if _, exists := existingAttributes[attribute]; exists {
			continue
		}
		existingAttributes[attribute] = struct{}{}
		identifier := factory.NewIdentifier(attribute)
		identifier.Virtual = true
		specifier := factory.NewImportSpecifier(false, nil, identifier)
		specifier.Virtual = true
		updated = append(updated, specifier)
	}
	if len(updated) == len(elements) {
		return declaration.AsNode()
	}
	named := factory.NewNamedImports(factory.NewNodeList(updated))
	clause := factory.UpdateImportClause(declaration.ImportClause.AsImportClause(), ast.KindUnknown, false, nil, named)
	return factory.UpdateImportDeclaration(declaration, declaration.Modifiers(), clause, declaration.ModuleSpecifier, declaration.Attributes)
}

func (p *KitImportProcessor) processKitDeclaration(
	factory *ast.NodeFactory,
	declaration *ast.ImportDeclaration,
	info kitJSONInfo,
	found bool,
	inEtsContext bool,
	kitMap map[string]*orderedStringSet,
	existingAttributes map[string]struct{},
) ([]*ast.Node, bool) {
	if !found || info.Symbols == nil {
		return nil, false
	}
	clause := declaration.ImportClause.AsImportClause()
	moduleName := declaration.ModuleSpecifier.Text()
	result := make([]*ast.Node, 0, 1)
	if name := clause.Name(); name != nil {
		symbol, ok := info.Symbols["default"]
		if !ok || !inEtsContext && strings.HasSuffix(symbol.Source, ".d.ets") {
			return nil, false
		}
		result = append(result, p.createKitImport(factory, clause.IsTypeOnly(), clause.IsLazy, name, symbol, declaration, name))
	}

	if clause.NamedBindings == nil {
		return result, true
	}
	working := declaration.AsNode()
	if components := kitMap[moduleName]; components != nil {
		working = p.supplementNamedBindings(factory, declaration, components, existingAttributes)
		delete(kitMap, moduleName)
	}
	workingClause := working.AsImportDeclaration().ImportClause.AsImportClause()
	for _, element := range workingClause.NamedBindings.AsNamedImports().Elements.Nodes {
		importName := element.Name().Text()
		if propertyName := element.AsImportSpecifier().PropertyName; propertyName != nil {
			importName = propertyName.Text()
		}
		if p.inKitWhiteList(moduleName, importName, inEtsContext) {
			return nil, false
		}
		symbol, ok := info.Symbols[importName]
		if !ok || !inEtsContext && strings.HasSuffix(symbol.Source, ".d.ets") ||
			clause.IsTypeOnly() && element.AsImportSpecifier().IsTypeOnly {
			return nil, false
		}
		result = append(result, p.createKitImport(
			factory,
			clause.IsTypeOnly() || element.AsImportSpecifier().IsTypeOnly,
			clause.IsLazy,
			element.Name(),
			symbol,
			declaration,
			element,
		))
	}
	return result, true
}

func (p *KitImportProcessor) inKitWhiteList(moduleName string, importName string, inEtsContext bool) bool {
	if symbols := kitErrorSymbolWhiteList[moduleName]; symbols != nil {
		if _, ok := symbols[importName]; ok {
			return true
		}
	}
	if !inEtsContext {
		if symbols := kitTSWarningWhiteList[moduleName]; symbols != nil {
			_, ok := symbols[importName]
			return ok
		}
	}
	return false
}

func (p *KitImportProcessor) createKitImport(
	factory *ast.NodeFactory,
	isType bool,
	isLazy bool,
	name *ast.Node,
	symbol kitSymbolInfo,
	original *ast.ImportDeclaration,
	originalSpecifier *ast.Node,
) *ast.Node {
	source := strings.TrimSuffix(strings.TrimSuffix(symbol.Source, ".d.ets"), ".d.ts")
	moduleSpecifier := factory.NewStringLiteral(source, ast.TokenFlagsNone)
	p.markKitVirtual(moduleSpecifier, original.ModuleSpecifier.Loc)

	var clause *ast.Node
	phaseModifier := ast.KindUnknown
	if isType {
		phaseModifier = ast.KindTypeKeyword
	}
	if symbol.Bindings == "default" {
		clause = factory.NewImportClause(phaseModifier, isLazy, name, nil)
	} else {
		var propertyName *ast.Node
		if !(ast.IsImportSpecifier(originalSpecifier) && originalSpecifier.AsImportSpecifier().PropertyName == nil && symbol.Bindings == name.Text()) {
			propertyName = factory.NewIdentifier(symbol.Bindings)
			p.markKitVirtual(propertyName, name.Loc)
		}
		specifier := factory.NewImportSpecifier(false, propertyName, name)
		p.markKitVirtual(specifier, originalSpecifier.Loc)
		named := factory.NewNamedImports(factory.NewNodeList([]*ast.Node{specifier}))
		p.markKitVirtual(named, originalSpecifier.Loc)
		clause = factory.NewImportClause(phaseModifier, isLazy, nil, named)
	}
	p.markKitVirtual(clause, originalSpecifier.Loc)
	declaration := factory.NewImportDeclaration(nil, clause, moduleSpecifier, nil)
	p.markKitVirtual(declaration, original.Loc)
	return declaration
}

func (p *KitImportProcessor) markKitVirtual(node *ast.Node, loc core.TextRange) {
	node.Virtual = true
	node.Flags |= ast.NodeFlagsKitImport
	node.Loc = loc
}
