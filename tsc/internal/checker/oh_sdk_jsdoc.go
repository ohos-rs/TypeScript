package checker

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

const (
	ohSinceWarning       = "The '{0}' API is supported since SDK version $SINCE1. However, the current compatible SDK version is $SINCE2.\n It is recommended to use apiAvailable to safeguard API compatibility."
	ohAvailableWarning   = "The '{0}' API is available since SDK version $SINCE1. However, the current compatible SDK version is $SINCE2.\n It is recommended to use apiAvailable to safeguard API compatibility."
	ohPermissionWarning  = "To use this API, you need to apply for the permissions: $DT"
	ohSystemAPIWarning   = "'{0}' is system api"
	ohTestWarning        = "'{0}' can only be used for testing directories "
	ohSyscapWarning      = "The system capacity of this api '{0}' is not supported on all devices"
	ohDeprecatedWarning  = "'{0}' has been deprecated."
	ohFormError          = "11706006#'{0}' can't support form application."
	ohCrossplatformError = "11706007#'{0}' can't support crossplatform application."
	ohFAModelError       = "11706008#This API is used only in FA Mode, but the current Mode is Stage."
	ohStageModelError    = "11706009#This API is used only in Stage Mode, but the current Mode is FA."
	ohAtomicServiceError = "11706010#'{0}' can't support atomicservice application."
	ohFindModuleWarning  = "Cannot find name '{0}'."
)

var (
	ohSinceFormatPattern            = regexp.MustCompile(`^(?:[1-9]\d{0,2}|[1-9]\d{0,2}\.\d{1,3}\.\d{1,3}|[1-9]\d{0,2}\.\d{1,3}\.\d{1,3}\([1-9]\d{0,2}\)|[1-9]\d?\.\d{1,2}\.\d{1,2})$`)
	ohVersionRangePattern           = regexp.MustCompile(`\[since (.*?)\]`)
	ohInvalidSuppressCommentPattern = regexp.MustCompile(`^//\s*@SuppressWarnings\s*(/+)`)
	ohSuppressCommentPattern        = regexp.MustCompile(`//\s*@SuppressWarnings\s`)
)

type ohSDKCommentSearch struct {
	node      *ast.Node
	isChain   bool
	chainNode *ast.Node
}

type ohSDKJSDocTag struct {
	name    string
	comment string
}

type ohSDKJSDocContract struct {
	tags []ohSDKJSDocTag
}

func (c *Checker) checkOHSDKIdentifierUse(node *ast.Node, symbol *ast.Symbol) {
	if node == nil || node.Virtual || symbol == nil || symbol.ValueDeclaration == nil {
		return
	}
	for _, declaration := range symbol.Declarations {
		c.checkOHSDKDeclarationUse(node, declaration)
	}
}

func (c *Checker) checkOHSDKPropertyUse(node *ast.Node, symbol *ast.Symbol) {
	if node == nil || symbol == nil || symbol.ValueDeclaration == nil || len(symbol.Declarations) >= 2 {
		return
	}
	for _, declaration := range symbol.Declarations {
		c.checkOHSDKDeclarationUse(node, declaration)
	}
}

func (c *Checker) checkOHSDKOverloadUse(node *ast.Node, declaration *ast.Node) {
	if node == nil || declaration == nil || !ast.IsPropertyAccessExpression(node.Expression()) {
		return
	}
	name := node.Expression().Name()
	symbol := c.getResolvedSymbolOrNil(node.Expression())
	if symbol == nil || len(symbol.Declarations) < 2 {
		return
	}
	c.checkOHSDKDeclarationUse(name, declaration)
}

func (c *Checker) checkOHSDKDeclarationUse(node *ast.Node, declaration *ast.Node) {
	if declaration == nil || node == nil {
		return
	}
	useFile := ast.GetSourceFileOfNode(node)
	declarationFile := ast.GetSourceFileOfNode(declaration)
	if useFile == nil || declarationFile == nil || slices.Contains(c.compilerOptions.OhSystemModules, filepath.Base(useFile.FileName())) {
		return
	}
	isProjectAvailable := c.isOHProjectFile(declarationFile.FileName()) && strings.Contains(declarationFile.Text(), "@Available")
	isSDK := c.isOHSDKDeclarationFile(declarationFile.FileName())
	if !isProjectAvailable && !isSDK {
		return
	}
	if !c.ohSDKNodeNeedsCheck(useFile.FileName(), declarationFile.FileName(), isProjectAvailable, isSDK) {
		return
	}
	if c.ohNeedsArkUIFindModuleWarning(useFile.FileName(), declarationFile.FileName()) {
		c.addOHSDKUseDiagnostic(node, ohFindModuleWarning, diagnostics.CategoryWarning)
	}
	contract := c.ohSDKJSDocContract(declaration)
	if isProjectAvailable {
		c.checkOHAvailableUse(node, declaration, contract)
		return
	}
	c.checkOHSinceUse(node, declaration, contract)
	c.checkOHSyscapUse(node, contract)
	c.checkOHPermissionUse(node, contract)
	c.checkOHPresenceTags(node, contract)
}

func (c *Checker) ohNeedsArkUIFindModuleWarning(useFile string, declarationFile string) bool {
	useFile = tspath.NormalizePath(useFile)
	if !strings.HasSuffix(useFile, ".ts") || strings.HasSuffix(useFile, ".d.ts") {
		return false
	}
	declarationFile = tspath.NormalizePath(declarationFile)
	baseName := filepath.Base(declarationFile)
	if baseName == "common_ts_ets_api.d.ts" || baseName == "global.d.ts" {
		return false
	}
	declarationDirectory := tspath.NormalizePath(filepath.Dir(declarationFile))
	for _, directory := range c.compilerOptions.OhArkUIDeclarationDirs {
		if declarationDirectory == tspath.NormalizePath(directory) {
			return true
		}
	}
	if c.compilerOptions.EtsLoaderPath == "" {
		return false
	}
	return declarationDirectory == tspath.NormalizePath(tspath.ResolvePath(c.compilerOptions.EtsLoaderPath, "declarations"))
}

func (c *Checker) isOHProjectFile(fileName string) bool {
	root := c.compilerOptions.OhProjectRootPath
	if root == "" {
		root = c.compilerOptions.OhProjectPath
	}
	return root != "" && strings.HasPrefix(tspath.NormalizePath(fileName), tspath.NormalizePath(root))
}

func (c *Checker) isOHSDKDeclarationFile(fileName string) bool {
	normalized := tspath.NormalizePath(fileName)
	if slices.ContainsFunc(c.compilerOptions.OhAllModulePaths, func(path string) bool {
		return tspath.NormalizePath(path) == normalized
	}) {
		return true
	}
	for _, config := range c.compilerOptions.OhSdkConfigs {
		for _, root := range config.ApiPaths {
			if root != "" && strings.HasPrefix(normalized, tspath.NormalizePath(root)+"/") {
				return true
			}
		}
	}
	for _, root := range c.compilerOptions.OhExternalApiPaths {
		if root != "" && strings.HasPrefix(normalized, tspath.NormalizePath(root)+"/") {
			return true
		}
	}
	for _, root := range c.compilerOptions.OhArkUIDeclarationDirs {
		if root != "" && strings.HasPrefix(normalized, tspath.NormalizePath(root)+"/") {
			return true
		}
	}
	if c.compilerOptions.EtsLoaderPath != "" {
		declarations := tspath.NormalizePath(tspath.ResolvePath(c.compilerOptions.EtsLoaderPath, "declarations"))
		return strings.HasPrefix(normalized, declarations+"/")
	}
	return false
}

func (c *Checker) ohSDKNodeNeedsCheck(useFile string, declarationFile string, projectAvailable bool, sdk bool) bool {
	if projectAvailable {
		return true
	}
	if !sdk {
		return false
	}
	if slices.ContainsFunc(c.compilerOptions.OhCardEntryFiles, func(path string) bool {
		return tspath.NormalizePath(path) == tspath.NormalizePath(useFile)
	}) || c.compilerOptions.OhCrossplatform.IsTrue() || c.compilerOptions.OhCompileMode != "" {
		return true
	}
	return c.compilerOptions.OhBundleType == "atomicService" && c.ohCompileSDKVersion() >= 11
}

func (c *Checker) ohSDKJSDocContract(declaration *ast.Node) ohSDKJSDocContract {
	contract := ohSDKJSDocContract{}
	// OpenHarmony checker.ts calls getJSDocTags(declaration). That helper owns
	// the nearest location's final JSDoc block; earlier adjacent blocks do not
	// contribute API-check tags.
	for _, tag := range getAllJSDocTags(declaration) {
		contract.tags = append(contract.tags, ohSDKJSDocTag{
			name:    tag.TagName().Text(),
			comment: scanner.GetTextOfJSDocComment(tag.CommentList()),
		})
	}
	return contract
}

func (contract ohSDKJSDocContract) first(names ...string) (ohSDKJSDocTag, bool) {
	for _, tag := range contract.tags {
		if slices.Contains(names, tag.name) {
			return tag, true
		}
	}
	return ohSDKJSDocTag{}, false
}

func (contract ohSDKJSDocContract) all(name string) []ohSDKJSDocTag {
	return core.Filter(contract.tags, func(tag ohSDKJSDocTag) bool { return tag.name == name })
}

func (c *Checker) addOHSDKUseDiagnostic(node *ast.Node, message string, category diagnostics.Category) {
	message = strings.ReplaceAll(message, "{0}", scanner.GetTextOfNode(node))
	c.addOHSDKDiagnostic(node, ohSDKCheckResult{valid: false, message: message, category: category})
}

func (c *Checker) ohCompatibleSDKVersion() string {
	if c.compilerOptions.OhOriginCompatibleSdkVersion != "" {
		return c.compilerOptions.OhOriginCompatibleSdkVersion
	}
	if c.compilerOptions.CompatibleSdkVersion == nil {
		return ""
	}
	value := *c.compilerOptions.CompatibleSdkVersion
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (c *Checker) ohCompileSDKVersion() int {
	if c.compilerOptions.CompileSdkVersion == nil {
		return 0
	}
	return int(*c.compilerOptions.CompileSdkVersion)
}

func compareOHSDKVersions(target string, required string) int {
	return compareOHPointVersions(strings.TrimSpace(target), strings.TrimSpace(required))
}

func parseOHVersionRange(comment string) (string, string, bool) {
	match := ohVersionRangePattern.FindStringSubmatch(comment)
	if len(match) != 2 {
		return "", "", false
	}
	parts := strings.Split(match[1], "-")
	if len(parts) != 2 {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func (c *Checker) ohVersionRangeIntersectsCompileSDK(comment string) bool {
	start, end, ok := parseOHVersionRange(comment)
	if !ok {
		return false
	}
	startValue := parseOHRangeVersion(start)
	endValue := parseOHRangeVersion(end)
	sdkValue := parseOHRangeVersion(strconv.Itoa(c.ohCompileSDKVersion()))
	minimum, maximum := min(startValue, endValue), max(startValue, endValue)
	return sdkValue >= minimum && sdkValue <= maximum
}

// api_check_utils.ts::parseVersion is intentionally narrower than the normal
// point-version comparator used by @since. Version ranges accept only a
// one/two-digit integer or a three-part one/two-digit M.S.F value for the
// OpenHarmony checker; unrecognized values compare as zero.
func parseOHRangeVersion(version string) int {
	version = strings.TrimSpace(version)
	if matched, _ := regexp.MatchString(`^\d{1,2}$`, version); matched {
		value, _ := strconv.Atoi(version)
		return value * 10000
	}
	if matched, _ := regexp.MatchString(`^\d{1,2}\.\d{1,2}\.\d{1,2}$`, version); !matched {
		return 0
	}
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	patch, _ := strconv.Atoi(parts[2])
	return major*10000 + minor*100 + patch
}

func (c *Checker) ohSDKCategory() diagnostics.Category {
	if c.compilerOptions.OhApiCompatibilityCheck == "error" {
		return diagnostics.CategoryError
	}
	return diagnostics.CategoryWarning
}

func (c *Checker) checkOHSinceUse(node *ast.Node, declaration *ast.Node, contract ohSDKJSDocContract) {
	tag, exists := contract.first("since")
	compatible := c.ohCompatibleSDKVersion()
	version := strings.TrimSpace(tag.comment)
	if !exists || !c.ohHasConfiguredCompatibleSDK() || compatible == "" || version == "" || !ohSinceFormatPattern.MatchString(version) || compareOHSDKVersions(compatible, version) >= 0 || !c.isOHProjectFile(ast.GetSourceFileOfNode(node).FileName()) {
		return
	}
	if c.ohSuppressesSDKWarning(node, declaration, "since", version) {
		return
	}
	message := strings.NewReplacer("$SINCE1", version, "$SINCE2", compatible).Replace(ohSinceWarning)
	if c.ohSDKCategory() == diagnostics.CategoryError {
		message = "11706011#" + message
	}
	c.addOHSDKUseDiagnostic(node, message, c.ohSDKCategory())
}

func (c *Checker) checkOHSyscapUse(node *ast.Node, contract ohSDKJSDocContract) {
	if len(c.compilerOptions.OhDeviceTypes) == 0 {
		return
	}
	tag, exists := contract.first("syscap")
	if !exists {
		tag.comment = ""
	}
	if slices.Contains(c.compilerOptions.OhSyscapIntersection, tag.comment) || c.ohSuppressesSDKWarning(node, nil, "syscap", tag.comment) {
		return
	}
	c.addOHSDKUseDiagnostic(node, ohSyscapWarning, diagnostics.CategoryWarning)
}

func (c *Checker) checkOHPermissionUse(node *ast.Node, contract ohSDKJSDocContract) {
	missing := make([]string, 0)
	for _, tag := range contract.all("permission") {
		comment := tag.comment
		if _, _, hasRange := parseOHVersionRange(comment); hasRange {
			if !c.ohVersionRangeIntersectsCompileSDK(comment) {
				continue
			}
			comment = strings.TrimSpace(ohVersionRangePattern.ReplaceAllString(comment, ""))
		}
		if comment == "" || (ohPermissionExpression{source: comment, granted: c.compilerOptions.OhRequestPermissions}).valid() {
			continue
		}
		if c.ohSuppressesSDKWarning(node, nil, "permission", "") {
			continue
		}
		missing = append(missing, comment)
	}
	if len(missing) == 0 {
		return
	}
	message := strings.Replace(ohPermissionWarning, "$DT", strings.Join(missing, " and "), 1)
	c.addOHSDKUseDiagnostic(node, message, diagnostics.CategoryWarning)
}

func (c *Checker) checkOHPresenceTags(node *ast.Node, contract ohSDKJSDocContract) {
	if tag, exists := contract.first("deprecated"); exists {
		_ = tag
		c.addOHSDKUseDiagnostic(node, ohDeprecatedWarning, diagnostics.CategoryWarning)
	}
	if tag, exists := contract.first("systemapi"); exists && (!strings.Contains(tag.comment, "[since ") || c.ohVersionRangeIntersectsCompileSDK(tag.comment)) {
		c.addOHSDKUseDiagnostic(node, ohSystemAPIWarning, diagnostics.CategoryWarning)
	}
	if tag, exists := contract.first("test"); exists && !c.isOHOhosTestFile(ast.GetSourceFileOfNode(node).FileName()) && (!strings.Contains(tag.comment, "[since ") || c.ohVersionRangeIntersectsCompileSDK(tag.comment)) {
		c.addOHSDKUseDiagnostic(node, ohTestWarning, diagnostics.CategoryWarning)
	}
	c.checkOHRequiredTag(node, contract, "form", c.isOHCardFile(ast.GetSourceFileOfNode(node).FileName()), ohFormError, diagnostics.CategoryError)
	c.checkOHRequiredTag(node, contract, "crossplatform", c.compilerOptions.OhCrossplatform.IsTrue(), ohCrossplatformError, core.IfElse(c.compilerOptions.OhIgnoreCrossplatformCheck.IsTrue(), diagnostics.CategoryWarning, diagnostics.CategoryError))
	c.checkOHRequiredTag(node, contract, "atomicservice", c.compilerOptions.OhBundleType == "atomicService" && c.ohCompileSDKVersion() >= 11, ohAtomicServiceError, diagnostics.CategoryError)
	if c.compilerOptions.OhCompileMode == "moduleJson" {
		if tag, exists := contract.first("famodelonly", "FAModelOnly"); exists && (!strings.Contains(tag.comment, "[since ") || c.ohVersionRangeIntersectsCompileSDK(tag.comment)) {
			c.addOHSDKUseDiagnostic(node, ohFAModelError, diagnostics.CategoryError)
		}
	} else if c.compilerOptions.OhCompileMode != "" {
		if tag, exists := contract.first("stagemodelonly", "StageModelOnly"); exists && (!strings.Contains(tag.comment, "[since ") || c.ohVersionRangeIntersectsCompileSDK(tag.comment)) {
			c.addOHSDKUseDiagnostic(node, ohStageModelError, diagnostics.CategoryError)
		}
	}
}

func (c *Checker) checkOHRequiredTag(node *ast.Node, contract ohSDKJSDocContract, tagName string, enabled bool, message string, category diagnostics.Category) {
	if !enabled {
		return
	}
	tag, exists := contract.first(tagName)
	unsupported := !exists
	if exists && strings.Contains(tag.comment, "[since ") {
		unsupported = !c.ohVersionRangeIntersectsCompileSDK(tag.comment)
	}
	if unsupported {
		c.addOHSDKUseDiagnostic(node, message, category)
	}
}

func (c *Checker) isOHCardFile(fileName string) bool {
	return slices.ContainsFunc(c.compilerOptions.OhCardEntryFiles, func(path string) bool {
		return tspath.NormalizePath(path) == tspath.NormalizePath(fileName)
	})
}

func (c *Checker) isOHOhosTestFile(fileName string) bool {
	if c.compilerOptions.OhModulePath == "" {
		return false
	}
	testRoot := tspath.NormalizePath(tspath.ResolvePath(c.compilerOptions.OhModulePath, "src", "ohosTest"))
	return strings.HasPrefix(tspath.NormalizePath(fileName), testRoot)
}

func (c *Checker) checkOHAvailableUse(node *ast.Node, declaration *ast.Node, contract ohSDKJSDocContract) {
	compatible := c.ohCompatibleSDKVersion()
	if !c.ohHasConfiguredCompatibleSDK() || !c.isOHProjectFile(ast.GetSourceFileOfNode(node).FileName()) || !c.isOHProjectFile(ast.GetSourceFileOfNode(declaration).FileName()) {
		return
	}
	key := filepath.Base(ast.GetSourceFileOfNode(node).FileName()) + "::" + strconv.Itoa(node.Pos()) + "::" + strconv.Itoa(node.End())
	if c.ohAvailableNodeChecks == nil {
		c.ohAvailableNodeChecks = make(map[string]struct{})
	}
	if _, exists := c.ohAvailableNodeChecks[key]; exists {
		return
	}
	c.ohAvailableNodeChecks[key] = struct{}{}
	version, ok := c.ohAvailableDecoratorVersion(declaration)
	if !ok || compatible == "" || compareOHSDKVersions(compatible, version.version) >= 0 {
		return
	}
	if c.ohSuppressesSDKWarning(node, declaration, "available", version.version) {
		return
	}
	// getAvailableCheckConfig uses the `since` tag as the presence gate. Once
	// the incompatibility callback succeeds, an existing @since prevents the
	// tagNameShouldExisted diagnostic from being collected.
	if _, exists := contract.first("since"); exists {
		return
	}
	message := strings.NewReplacer("$SINCE1", version.version, "$SINCE2", compatible).Replace(ohAvailableWarning)
	if c.ohSDKCategory() == diagnostics.CategoryError {
		message = "11706012#" + message
	}
	c.addOHSDKUseDiagnostic(node, message, c.ohSDKCategory())
}

func (c *Checker) ohHasConfiguredCompatibleSDK() bool {
	return c.compilerOptions.CompatibleSdkVersion != nil && *c.compilerOptions.CompatibleSdkVersion != 0
}

func (c *Checker) ohAvailableDecoratorVersion(node *ast.Node) (ohParsedVersion, bool) {
	for current := node; current != nil; current = current.Parent {
		for _, modifier := range current.ModifierNodes() {
			if !ast.IsDecorator(modifier) {
				continue
			}
			expression := modifier.Expression()
			if !ast.IsCallExpression(expression) || !ast.IsIdentifier(expression.Expression()) || expression.Expression().Text() != "Available" || len(expression.Arguments()) == 0 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
				continue
			}
			for _, property := range expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes {
				if !ast.IsPropertyAssignment(property) || property.Name() == nil {
					continue
				}
				name, ok := ast.TryGetTextOfPropertyName(property.Name())
				if !ok || name != "minApiVersion" {
					continue
				}
				initializer := property.Initializer()
				if !ast.IsStringLiteral(initializer) && !ast.IsNumericLiteral(initializer) {
					continue
				}
				version := parseOHAvailableVersion(initializer.Text())
				if validateOHAvailableVersion(version, c.ohRuntimeOS()).valid {
					return version, true
				}
			}
		}
	}
	return ohParsedVersion{}, false
}

func (c *Checker) ohSuppressesSDKWarning(node *ast.Node, declaration *ast.Node, warning string, value string) bool {
	if c.ohSuppressWarningsAnnotation(node, warning) || c.ohSuppressWarningsComment(node, warning) {
		return true
	}
	switch warning {
	case "since":
		return c.ohUseInTry(node) || c.ohUseUnderUndefinedCheck(node) || c.ohSDKWhitelist(declaration) ||
			c.ohUseUnderAvailable(node, value) || c.ohUseUnderSDKGuard(node, value)
	case "available":
		return c.ohUseUnderAvailable(node, value) || c.ohUseUnderSDKGuard(node, value)
	case "syscap":
		return c.ohUseUnderCanIUse(node, value)
	default:
		return false
	}
}

func (c *Checker) ohSuppressWarningsAnnotation(node *ast.Node, warning string) bool {
	rule := ""
	switch warning {
	case "since", "available":
		rule = "SuppressWarningsType.COMPATIBILITY"
	case "syscap":
		rule = "SuppressWarningsType.SYSCAP"
	case "permission":
		rule = "SuppressWarningsType.PERMISSION"
	}
	if rule == "" {
		return false
	}
	for current := node; current != nil; current = current.Parent {
		foundSuppressWarnings := false
		for _, modifier := range current.ModifierNodes() {
			if !ast.IsDecorator(modifier) {
				continue
			}
			expression := modifier.Expression()
			if !ast.IsCallExpression(expression) || !ast.IsIdentifier(expression.Expression()) || expression.Expression().Text() != "SuppressWarnings" || len(expression.Arguments()) == 0 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
				continue
			}
			foundSuppressWarnings = true
			properties := expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes
			if len(properties) == 0 || !ast.IsPropertyAssignment(properties[0]) || !ast.IsArrayLiteralExpression(properties[0].Initializer()) {
				continue
			}
			for _, element := range properties[0].Initializer().AsArrayLiteralExpression().Elements.Nodes {
				if strings.Contains(rule, scanner.GetTextOfNode(element)) {
					return true
				}
			}
		}
		// api_validate_node.ts::getTagDecoratorFromNode stops at the nearest
		// node that declares any SuppressWarnings decorator. An inner decorator
		// therefore shadows outer rules even when it names another warning kind.
		if foundSuppressWarnings {
			return false
		}
	}
	return false
}

func (c *Checker) ohSuppressWarningsComment(node *ast.Node, warning string) bool {
	warningName := warning
	if warning == "since" || warning == "available" {
		warningName = "compatibility"
	}
	if warningName != "compatibility" && warningName != "syscap" && warningName != "permission" || !ast.IsIdentifier(node) {
		return false
	}
	file := ast.GetSourceFileOfNode(node)
	if file == nil {
		return false
	}
	search := c.ohSDKFindCommentStatement(node, warningName)
	search = c.ohSDKFindOuterChain(search, warningName)
	if search.node == nil {
		return false
	}
	commentPosition := search.node.Pos()
	if search.isChain && search.chainNode != nil && ast.IsPropertyAccessExpression(search.chainNode.Parent) {
		commentPosition = search.chainNode.Parent.Expression().End()
	}
	comments := make([]string, 0)
	for comment := range scanner.GetLeadingCommentRanges(&ast.NodeFactory{}, file.Text(), commentPosition) {
		comments = append(comments, file.Text()[comment.Pos():comment.End()])
	}
	for _, comment := range comments {
		if strings.HasPrefix(comment, "/*") || ohInvalidSuppressCommentPattern.MatchString(comment) {
			return false
		}
	}
	for _, comment := range comments {
		if ohSuppressCommentPattern.MatchString(comment) && ohCommentWord(comment, warningName) {
			return true
		}
	}
	return false
}

// api_validate_node.ts::findNodeParentStatement preserves comments attached to
// an individual link in ArkUI's fluent call syntax. Walking straight to the
// containing statement loses those ranges because they live between nested
// property-access nodes.
func (c *Checker) ohSDKFindCommentStatement(node *ast.Node, warningName string) ohSDKCommentSearch {
	search := ohSDKCommentSearch{node: node, chainNode: node}
	for search.node != nil && !ast.IsStatement(search.node) && !ast.IsPropertyDeclaration(search.node) && search.node.Parent != nil {
		if ast.IsPropertyAccessExpression(search.node) && ast.IsCallExpression(search.node.Expression()) {
			search.isChain = strings.Contains(scanner.GetTextOfNode(search.node), "SuppressWarnings")
			if c.ohSDKNodeHasSuppressComment(search.node, warningName) {
				search.chainNode = search.node
				break
			}
		}
		search.node = search.node.Parent
	}
	return search
}

// api_validate_node.ts::getChainCallNode additionally handles an API use in an
// arrow-function callback that is itself a link of a fluent call chain.
func (c *Checker) ohSDKFindOuterChain(search ohSDKCommentSearch, warningName string) ohSDKCommentSearch {
	backup := search.node
	if search.isChain || search.node == nil || search.node.Parent == nil || search.node.Parent.Parent == nil ||
		!ast.IsBlock(search.node.Parent) || !ast.IsArrowFunction(search.node.Parent.Parent) {
		return search
	}
	for search.node != nil && search.node.Parent != nil {
		if ast.IsArrowFunction(search.node) && ast.IsCallExpression(search.node.Parent) && ast.IsPropertyAccessExpression(search.node.Parent.Expression()) {
			if c.ohSDKNodeHasSuppressComment(backup, warningName) {
				search.node = backup
				break
			}
			expression := search.node.Parent.Expression()
			search.isChain = strings.Contains(scanner.GetTextOfNode(expression), "SuppressWarnings")
			search.chainNode = expression.Expression()
			search.node = expression.Expression()
			break
		}
		search.node = search.node.Parent
	}
	return search
}

func (c *Checker) ohSDKNodeHasSuppressComment(node *ast.Node, warningName string) bool {
	if node == nil {
		return false
	}
	file := ast.GetSourceFileOfNode(node)
	if file == nil {
		return false
	}
	comments := make([]string, 0)
	for comment := range scanner.GetLeadingCommentRanges(&ast.NodeFactory{}, file.Text(), node.Pos()) {
		comments = append(comments, file.Text()[comment.Pos():comment.End()])
	}
	for _, comment := range comments {
		if strings.HasPrefix(comment, "/*") || ohInvalidSuppressCommentPattern.MatchString(comment) {
			return false
		}
	}
	for _, comment := range comments {
		if ohSuppressCommentPattern.MatchString(comment) && ohCommentWord(comment, warningName) {
			return true
		}
	}
	return false
}

func ohCommentWord(comment string, word string) bool {
	for _, field := range strings.FieldsFunc(comment, func(r rune) bool { return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == ',' }) {
		if field == word {
			return true
		}
	}
	return false
}

func (c *Checker) ohUseInTry(node *ast.Node) bool {
	for current := node.Parent; current != nil; current = current.Parent {
		if ast.IsTryStatement(current) && current.AsTryStatement().TryBlock != nil && node.Pos() >= current.AsTryStatement().TryBlock.Pos() {
			return true
		}
	}
	return false
}

func (c *Checker) ohUseUnderUndefinedCheck(node *ast.Node) bool {
	target := ohPrimaryNodeName(node)
	if target == "" {
		return false
	}
	for current := node.Parent; current != nil; current = current.Parent {
		if !ast.IsIfStatement(current) || !ast.IsBinaryExpression(current.Expression()) {
			continue
		}
		expression := current.Expression().AsBinaryExpression()
		if expression.OperatorToken.Kind != ast.KindExclamationEqualsToken && expression.OperatorToken.Kind != ast.KindExclamationEqualsEqualsToken {
			continue
		}
		leftUndefined := ast.IsIdentifier(expression.Left) && expression.Left.Text() == "undefined"
		rightUndefined := ast.IsIdentifier(expression.Right) && expression.Right.Text() == "undefined"
		if ohPrimaryNodeName(expression.Left) == target && rightUndefined || leftUndefined && ohPrimaryNodeName(expression.Right) == target {
			return true
		}
	}
	return false
}

func ohPrimaryNodeName(node *ast.Node) string {
	if node == nil {
		return ""
	}
	switch node.Kind {
	case ast.KindIdentifier:
		return node.Text()
	case ast.KindCallExpression:
		return ohPrimaryNodeName(node.Expression())
	case ast.KindPropertyAccessExpression:
		return node.Name().Text()
	}
	return ""
}

func (c *Checker) ohSDKWhitelist(declaration *ast.Node) bool {
	if declaration == nil || declaration.Name() == nil {
		return false
	}
	whitelist := map[string][]string{
		"@arkts.lang.d.ets":     {"RetentionPolicy", "Retention", "SOURCE", "BYTECODE"},
		"@ohos.deviceInfo.d.ts": {"apiAvailable"},
	}
	fileName := tspath.NormalizePath(ast.GetSourceFileOfNode(declaration).FileName())
	for _, root := range c.compilerOptions.OhGlobalModulePaths {
		relative, err := filepath.Rel(root, fileName)
		if err == nil && slices.Contains(whitelist[tspath.NormalizePath(relative)], declaration.Name().Text()) {
			return true
		}
	}
	return false
}

func (c *Checker) ohUseUnderAvailable(node *ast.Node, required string) bool {
	if !c.isOHProjectFile(ast.GetSourceFileOfNode(node).FileName()) || !strings.Contains(ast.GetSourceFileOfNode(node).Text(), "@Available") {
		return false
	}
	version, ok := c.ohAvailableDecoratorVersion(node)
	return ok && compareOHSDKVersions(version.version, required) >= 0
}

func (c *Checker) ohUseUnderSDKGuard(node *ast.Node, required string) bool {
	file := ast.GetSourceFileOfNode(node)
	if file == nil || !strings.Contains(file.Text(), "deviceInfo") {
		return false
	}
	for current := node.Parent; current != nil; current = current.Parent {
		if !ast.IsIfStatement(current) || current.AsIfStatement().ThenStatement == nil || node.Pos() < current.AsIfStatement().ThenStatement.Pos() || node.End() > current.AsIfStatement().ThenStatement.End() {
			continue
		}
		condition := current.Expression()
		if c.ohApiAvailableGuard(condition, required) || c.ohSDKVersionGuard(condition, required) {
			return true
		}
	}
	return false
}

func (c *Checker) ohApiAvailableGuard(condition *ast.Node, required string) bool {
	if !ast.IsCallExpression(condition) || len(condition.Arguments()) != 1 {
		return false
	}
	conditionText := scanner.GetTextOfNode(condition)
	matchedAPI := ""
	for _, candidate := range []string{"distributionOSApiVersion", "sdkApiVersion", "apiAvailable"} {
		if strings.Contains(conditionText, candidate) {
			matchedAPI = candidate
			break
		}
	}
	if matchedAPI == "" || c.ohRuntimeOS() == ohRuntimeOS && matchedAPI == "distributionOSApiVersion" {
		return false
	}
	argument := condition.Arguments()[0]
	if !c.validateOHApiAvailableArgument(argument).valid {
		return false
	}
	argumentText := strings.NewReplacer("'", "", "\"", "", "`", "", "|", "").Replace(strings.TrimSpace(scanner.GetTextOfNode(argument)))
	if !ohAvailableFormatPattern.MatchString(argumentText) {
		return false
	}
	return compareOHSDKVersions(argumentText, required) >= 0
}

func (c *Checker) ohSDKVersionGuard(condition *ast.Node, required string) bool {
	if parts := strings.Split(required, "."); len(parts) >= 3 && parseDecimal(parts[0]) > ohMSFIntegerVersion {
		return false
	}
	if !ast.IsBinaryExpression(condition) {
		return false
	}
	expression := condition.AsBinaryExpression()
	apiSide, valueSide, apiOnLeft := expression.Left, expression.Right, true
	apiName := ohPrimaryNodeName(apiSide)
	if apiName != "sdkApiVersion" && apiName != "distributionOSApiVersion" {
		apiSide, valueSide, apiOnLeft = expression.Right, expression.Left, false
		apiName = ohPrimaryNodeName(apiSide)
	}
	if apiName != "sdkApiVersion" && apiName != "distributionOSApiVersion" || apiName == "distributionOSApiVersion" && c.ohRuntimeOS() == ohRuntimeOS || !ast.IsPropertyAccessExpression(apiSide) || !c.isOHDeviceInfoRoot(apiSide.Expression()) {
		return false
	}
	value, ok := c.ohSDKGuardValue(valueSide)
	if !ok {
		return false
	}
	operator := expression.OperatorToken.Kind
	if !apiOnLeft {
		switch operator {
		case ast.KindGreaterThanToken:
			operator = ast.KindLessThanToken
		case ast.KindLessThanToken:
			operator = ast.KindGreaterThanToken
		case ast.KindGreaterThanEqualsToken:
			operator = ast.KindLessThanEqualsToken
		case ast.KindLessThanEqualsToken:
			operator = ast.KindGreaterThanEqualsToken
		}
	}
	assigned := value
	switch operator {
	case ast.KindGreaterThanToken:
		assigned++
	case ast.KindGreaterThanEqualsToken, ast.KindEqualsEqualsToken, ast.KindEqualsEqualsEqualsToken:
	default:
		return false
	}
	return compareOHSDKVersions(strconv.Itoa(assigned), required) >= 0
}

func (c *Checker) ohSDKGuardValue(node *ast.Node) (int, bool) {
	if ast.IsNumericLiteral(node) {
		value, err := strconv.ParseFloat(node.Text(), 64)
		if err == nil {
			return int(value + 0.999999999), true
		}
	}
	if ast.IsIdentifier(node) {
		symbol := c.getResolvedSymbolOrNil(node)
		if symbol != nil && symbol.ValueDeclaration != nil && (ast.IsVariableDeclaration(symbol.ValueDeclaration) || ast.IsBindingElement(symbol.ValueDeclaration)) {
			initializer := symbol.ValueDeclaration.Initializer()
			if ast.IsStringLiteral(initializer) {
				value, err := strconv.ParseFloat(initializer.Text(), 64)
				if err == nil {
					return int(value + 0.999999999), true
				}
			}
			return c.ohSDKGuardValue(initializer)
		}
	}
	return 0, false
}

func (c *Checker) isOHDeviceInfoRoot(node *ast.Node) bool {
	for ast.IsPropertyAccessExpression(node) {
		node = node.Expression()
	}
	if !ast.IsIdentifier(node) {
		return false
	}
	symbol := c.getResolvedSymbolOrNil(node)
	if symbol == nil {
		return false
	}
	if symbol.Flags&ast.SymbolFlagsAlias != 0 {
		symbol = c.resolveAlias(symbol)
	}
	for _, declaration := range symbol.Declarations {
		if strings.Contains(strings.ToLower(tspath.NormalizePath(ast.GetSourceFileOfNode(declaration).FileName())), strings.ToLower(ohDeviceInfoFileName)) {
			return true
		}
	}
	return false
}

func (c *Checker) ohUseUnderCanIUse(node *ast.Node, syscap string) bool {
	file := ast.GetSourceFileOfNode(node)
	if file == nil || !strings.Contains(file.Text(), "canIUse(") {
		return false
	}
	for current := node.Parent; current != nil; current = current.Parent {
		if !ast.IsIfStatement(current) || !ast.IsCallExpression(current.Expression()) || !ast.IsIdentifier(current.Expression().Expression()) || current.Expression().Expression().Text() != "canIUse" || len(current.Expression().Arguments()) != 1 {
			continue
		}
		argument := current.Expression().Arguments()[0]
		if ast.IsStringLiteral(argument) && argument.Text() == syscap {
			return true
		}
		if ast.IsIdentifier(argument) {
			symbol := c.getResolvedSymbolOrNil(argument)
			if symbol != nil && symbol.ValueDeclaration != nil && ast.IsVariableDeclaration(symbol.ValueDeclaration) && ast.IsStringLiteral(symbol.ValueDeclaration.Initializer()) && symbol.ValueDeclaration.Initializer().Text() == syscap {
				return true
			}
		}
	}
	return false
}

type ohPermissionExpression struct {
	source  string
	granted []string
}

func (p ohPermissionExpression) valid() bool {
	queue := p.tokens()
	result := ohPermissionCalculation{currentPermissionMatch: true}
	p.validate(queue, &result)
	return result.valid
}

type ohPermissionToken int

const (
	ohPermissionInitial ohPermissionToken = iota
	ohPermissionAnd
	ohPermissionOr
)

type ohPermissionCalculation struct {
	valid                  bool
	currentToken           ohPermissionToken
	finish                 bool
	currentPermissionMatch bool
}

type ohPermissionGroup struct {
	queue              []string
	includeParentheses bool
}

func (p ohPermissionExpression) tokens() []string {
	items := make([]string, 0)
	for _, item := range strings.Split(p.source, " ") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}
	for _, separator := range []string{"(", ")", "\n"} {
		next := make([]string, 0, len(items))
		for _, item := range items {
			if !strings.Contains(item, separator) {
				next = append(next, item)
				continue
			}
			for _, part := range strings.Split(item, separator) {
				part = strings.TrimSpace(part)
				if part == "" {
					next = append(next, separator)
				} else {
					next = append(next, part)
				}
			}
		}
		items = next
	}
	return items
}

func (p ohPermissionExpression) validate(queue []string, result *ohPermissionCalculation) {
	if slices.Contains(queue, "(") || slices.Contains(queue, ")") {
		queue = p.flattenGroups(p.groups(queue), *result)
	}
	p.validateAtoms(queue, result)
}

func (p ohPermissionExpression) groups(queue []string) []ohPermissionGroup {
	depth := 0
	groups := make([]ohPermissionGroup, 0)
	current := ohPermissionGroup{}
	for index, item := range queue {
		switch item {
		case "(":
			if depth == 0 {
				groups = append(groups, current)
				current = ohPermissionGroup{queue: []string{item}, includeParentheses: true}
			} else {
				current.queue = append(current.queue, item)
			}
			depth++
		case ")":
			depth--
			current.queue = append(current.queue, item)
			if depth == 0 {
				groups = append(groups, current)
				current = ohPermissionGroup{}
			}
		default:
			current.queue = append(current.queue, item)
			if index == len(queue)-1 {
				groups = append(groups, current)
			}
		}
	}
	return groups
}

func (p ohPermissionExpression) flattenGroups(groups []ohPermissionGroup, initial ohPermissionCalculation) []string {
	queue := make([]string, 0)
	for _, group := range groups {
		if !group.includeParentheses {
			queue = append(queue, group.queue...)
			continue
		}
		inner := []string{}
		if len(group.queue) >= 2 {
			inner = group.queue[1 : len(group.queue)-1]
		}
		result := initial
		p.validate(inner, &result)
		if result.valid {
			queue = append(queue, "")
		} else {
			queue = append(queue, "NA")
		}
	}
	return queue
}

func (p ohPermissionExpression) validateAtoms(queue []string, result *ohPermissionCalculation) {
	if result.finish {
		return
	}
	if len(queue) == 0 {
		result.currentPermissionMatch = false
		result.valid = false
		result.finish = true
		return
	}
	item := queue[0]
	switch item {
	case "and":
		result.currentToken = ohPermissionAnd
	case "or":
		result.currentToken = ohPermissionOr
	default:
		matches := item == "" || slices.Contains(p.granted, item)
		switch result.currentToken {
		case ohPermissionOr:
			if !result.currentPermissionMatch && !matches {
				result.currentPermissionMatch = false
			} else {
				result.currentPermissionMatch = true
			}
		case ohPermissionAnd:
			if !result.currentPermissionMatch || !matches {
				result.currentPermissionMatch = false
			} else {
				result.currentPermissionMatch = matches
			}
		default:
			result.currentPermissionMatch = matches
		}
	}
	if len(queue) > 1 {
		p.validateAtoms(queue[1:], result)
		return
	}
	result.valid = result.currentPermissionMatch
	result.finish = true
}
