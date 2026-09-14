package parser

import (
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"strings"
)

// struct is contextual: existing TypeScript identifiers remain legal in ETS.
func (p *Parser) isStructDeclaration() bool {
	return p.scriptKind == core.ScriptKindETS && p.token == ast.KindIdentifier &&
		p.scanner.TokenValue() == "struct" && p.lookAhead((*Parser).nextTokenStartsStructTail)
}

func (p *Parser) nextTokenStartsStructTail() bool {
	p.nextToken()
	return (p.isIdentifier() || p.token == ast.KindOpenBraceToken) && !p.hasPrecedingLineBreak()
}

// OH parser.ts::hasParamAndNoOnceDecorator/hasEnvDecorator and
// parseClassElement add readonly before ordinary class-member checking.
func (p *Parser) addArkUIReadonly(modifiers *ast.ModifierList) *ast.ModifierList {
	if !p.arkUIStruct || modifiers == nil {
		return modifiers
	}
	param, once, env := false, false, false
	for _, modifier := range modifiers.Nodes {
		if modifier.Kind == ast.KindReadonlyKeyword {
			return modifiers
		}
		if !ast.IsDecorator(modifier) {
			continue
		}
		expr := modifier.Expression()
		if ast.IsIdentifier(expr) {
			param = param || expr.Text() == "Param"
			once = once || expr.Text() == "Once"
		} else if ast.IsCallExpression(expr) && ast.IsIdentifier(expr.Expression()) {
			env = env || expr.Expression().Text() == "Env" || expr.Expression().Text() == "CustomEnv"
		}
	}
	if !env && (!param || once) {
		return modifiers
	}
	readonly := p.finishEtsVirtualNode(p.factory.NewModifier(ast.KindReadonlyKeyword), p.nodePos())
	nodes := append(modifiers.Nodes, readonly)
	return p.newModifierList(modifiers.Loc, p.nodeSliceArena.Clone(nodes))
}

// OH parseDeclaration/parseFunctionDeclaration/parseMethodDeclaration keep
// component context distinct from configured build/builder callback context.
func (p *Parser) enterEtsFunction(modifiers *ast.ModifierList, methodName string, method bool) func() {
	ui, styles, callback := p.arkUI, p.arkUIStyles, p.arkUICallback
	build, styleDecl, generic := p.arkUIBuild, p.arkUIStyleDeclaration, p.arkUIStyleGeneric
	stateRoot := p.etsStateRoot
	p.etsStateRoot = ""
	p.arkUI, p.arkUIStyles, p.arkUIBuild = false, false, false
	p.arkUIStyleDeclaration, p.arkUIStyleGeneric = nil, false
	if p.scriptKind == core.ScriptKindETS {
		configuredBuilder := p.hasEtsBuilder(modifiers, false)
		p.arkUIBuild = configuredBuilder || method && methodName == "build" && p.opts.Ets.Render.Method.Contains("build")
		p.arkUI = p.hasEtsBuilder(modifiers, true)
		if method {
			p.arkUI = p.arkUIStruct && (methodName == "build" || methodName == "pageTransition" || p.arkUI)
		}
		hasExtend := false
		if !method {
			last := ""
			if modifiers != nil {
				for _, decorator := range modifiers.Nodes {
					if !ast.IsDecorator(decorator) || !ast.IsCallExpression(decorator.Expression()) {
						continue
					}
					call := decorator.Expression()
					if ast.IsIdentifier(call.Expression()) && p.opts.Ets.Extend.Decorator.Contains(call.Expression().Text()) {
						hasExtend = true
						if len(call.Arguments()) > 0 && ast.IsIdentifier(call.Arguments()[0]) {
							last = call.Arguments()[0].Text()
						}
					}
				}
			}
			for declaration := range p.opts.Ets.Extend.Components.Values() {
				if last != "" && declaration.Name == last {
					p.arkUIStyleDeclaration = &declaration
				}
			}
		}
		// OH parseDeclaration uses mutually exclusive Extend/Styles/Builder
		// branches, even when an Extend decorator has no matching component.
		if hasExtend {
			p.arkUI = false
		}
		if !hasExtend && (!method || p.arkUIStruct) {
			if name, ok := p.opts.Ets.Styles.Decorator.Get(); ok && ast.HasArkUIBareDecorator(modifiers, name) {
				if !method {
					p.arkUI = false
				}
				if declaration, present := p.opts.Ets.Styles.Component.Get(); present {
					p.arkUIStyleDeclaration, p.arkUIStyleGeneric = &declaration, true
				}
			}
		}
		p.arkUIStyles = p.arkUIStyleDeclaration != nil
	}
	p.arkUICallback = false
	return func() {
		p.arkUI, p.arkUIStyles, p.arkUICallback = ui, styles, callback
		p.arkUIBuild, p.arkUIStyleDeclaration, p.arkUIStyleGeneric = build, styleDecl, generic
		p.etsStateRoot = stateRoot
	}
}

func (p *Parser) hasEtsBuilder(modifiers *ast.ModifierList, fallback bool) bool {
	if p.opts.Ets.Render.Decorator.IsZero() && fallback {
		return ast.HasArkUIBareDecorator(modifiers, "Builder", "LocalBuilder")
	}
	for name := range p.opts.Ets.Render.Decorator.Values() {
		if ast.HasArkUIBareDecorator(modifiers, name) {
			return true
		}
	}
	return false
}

func (p *Parser) etsVirtualIdentifier(name string, pos int) *ast.Node {
	node := p.factory.NewIdentifier(name)
	return p.finishEtsVirtualNode(node, pos)
}

func (p *Parser) finishEtsVirtualNode(node *ast.Node, pos int) *ast.Node {
	p.finishNodeWithEnd(node, pos, pos)
	node.Virtual = true
	return node
}

func (p *Parser) parseEtsFunctionTypeParameters() *ast.NodeList {
	if !p.arkUIStyleGeneric {
		return p.parseTypeParameters()
	}
	pos := p.nodePos()
	parameter := p.parseTypeParameterWorker(true)
	return p.newNodeList(core.NewTextRange(pos, p.nodePos()), p.nodeSliceArena.NewSlice1(parameter))
}

func (p *Parser) parseEtsFunctionReturnType() *ast.Node {
	pos := p.nodePos()
	result := p.parseReturnType(ast.KindColonToken, false)
	if result == nil && p.arkUIStyleDeclaration != nil {
		result = p.factory.NewTypeReferenceNode(p.etsVirtualIdentifier(p.arkUIStyleDeclaration.Type, pos), nil)
		p.finishEtsVirtualNode(result, pos)
	}
	return result
}

// OH parseStructMembers: a real virtual constructor supplies the type system
// with the property bag and LocalStorage parameters. Never infer the bag from
// initializers or add Require-based mandatory fields.
func (p *Parser) addEtsStructConstructor(members *ast.NodeList, pos int) *ast.NodeList {
	var properties []*ast.Node
	for _, member := range members.Nodes {
		if !ast.IsPropertyDeclaration(member) {
			continue
		}
		// The virtual signature mirrors the declaration, decorators included, so
		// `@Require @Prop` survives for the build transform that enforces it.
		var modifiers []*ast.Node
		for _, modifier := range member.ModifierNodes() {
			modifiers = append(modifiers, p.factory.DeepCloneReparse(modifier))
		}
		var modifierList *ast.ModifierList
		if len(modifiers) != 0 {
			modifierList = p.factory.NewModifierList(modifiers)
		}
		var typ *ast.Node
		if member.Type() != nil {
			typ = p.factory.DeepCloneReparse(member.Type())
		}
		property := p.factory.NewPropertySignatureDeclaration(modifierList, p.factory.DeepCloneReparse(member.Name()), p.factory.NewToken(ast.KindQuestionToken), typ, nil)
		properties = append(properties, p.finishEtsVirtualNode(property, 0))
	}
	var parameters []*ast.Node
	if len(properties) != 0 {
		typ := p.finishEtsVirtualNode(p.factory.NewTypeLiteralNode(p.newNodeList(core.NewTextRange(0, 0), properties)), 0)
		parameters = append(parameters, p.finishEtsVirtualNode(p.factory.NewParameterDeclaration(nil, nil, p.etsVirtualIdentifier("value", 0), p.factory.NewToken(ast.KindQuestionToken), typ, nil), 0))
	}
	storage := p.finishEtsVirtualNode(p.factory.NewTypeReferenceNode(p.etsVirtualIdentifier("LocalStorage", 0), nil), 0)
	parameters = append(parameters, p.finishEtsVirtualNode(p.factory.NewParameterDeclaration(nil, nil, p.etsVirtualIdentifier("##storage", 0), p.factory.NewToken(ast.KindQuestionToken), storage, nil), 0))
	body := p.finishEtsVirtualNode(p.factory.NewBlock(p.newNodeList(core.NewTextRange(0, 0), nil), false), 0)
	constructor := p.finishEtsVirtualNode(p.factory.NewConstructorDeclaration(nil, nil, p.newNodeList(core.NewTextRange(0, 0), parameters), nil, nil, body), pos)
	return p.newNodeList(members.Loc, append([]*ast.Node{constructor}, members.Nodes...))
}

func (p *Parser) etsStructHeritage(name string) *ast.NodeList {
	pos := p.nodePos()
	expression := p.finishEtsVirtualNode(p.factory.NewExpressionWithTypeArguments(p.etsVirtualIdentifier(name, pos), nil), pos)
	clause := p.finishEtsVirtualNode(p.factory.NewHeritageClause(ast.KindExtendsKeyword, p.newNodeList(core.NewTextRange(pos, pos), p.nodeSliceArena.NewSlice1(expression))), pos)
	return p.newNodeList(core.NewTextRange(pos, pos), p.nodeSliceArena.NewSlice1(clause))
}

func (p *Parser) isArkUIIteration(node *ast.Node) bool {
	if ast.IsIdentifier(node) {
		return p.opts.Ets.SyntaxComponents.ParamsUICallback.Contains(node.Text())
	}
	if ast.IsPropertyAccessExpression(node) {
		root := node.Expression()
		for ast.IsCallExpression(root) || ast.IsPropertyAccessExpression(root) {
			root = root.Expression()
		}
		if ast.IsIdentifier(root) {
			for component := range p.opts.Ets.SyntaxComponents.AttrUICallback.Values() {
				if component.Name == root.Text() {
					return component.Attributes.Contains(node.Name().Text())
				}
			}
		}
	}
	return false
}

// OH parseCallExpressionRest/getRootComponent: virtual arguments are AST
// inputs to inference, not a checker-side fluent-return-type override.
func (p *Parser) etsAttributeArguments(expression *ast.Node, pos int, original *ast.NodeList) (*ast.NodeList, bool) {
	if !p.arkUIBuild && !p.arkUIStyles || !ast.IsPropertyAccessExpression(expression) {
		return original, false
	}
	attribute := expression.Name().Text()
	stateProperty := attribute == p.opts.Ets.Styles.Property.Or("")
	rootName, component, callbackRoot := "", false, false
	for node := expression; node != nil; {
		if ast.IsEtsComponentExpression(node) {
			rootName, component = node.Expression().Text(), true
			break
		}
		if ast.IsCallExpression(node) && ast.IsIdentifier(node.Expression()) {
			rootName = "Common"
			for entry := range p.opts.Ets.SyntaxComponents.AttrUICallback.Values() {
				if entry.Name == node.Expression().Text() {
					rootName, callbackRoot = entry.Name, true
					break
				}
			}
			break
		}
		if !ast.IsCallExpression(node) && !ast.IsPropertyAccessExpression(node) {
			break
		}
		node = node.Expression()
	}
	virtualType := ""
	if rootName != "" {
		p.etsStateRoot = ""
		if stateProperty {
			p.etsStateRoot = rootName
		}
		callback := false
		if callbackRoot {
			for entry := range p.opts.Ets.SyntaxComponents.AttrUICallback.Values() {
				if entry.Name == rootName {
					callback = entry.Attributes.Contains(attribute)
					break
				}
			}
		}
		if callback {
			p.arkUICallback = true
		} else if rootName == "WithEnv" && (attribute == "env" || attribute == "customEnv") {
			// OH deliberately leaves these environment calls without virtual arguments.
		} else if component {
			virtualType = rootName + "Attribute"
		}
	} else if p.etsStateRoot != "" {
		virtualType = p.etsStateRoot + "Attribute"
	} else if p.arkUIStyles && stateProperty {
		for node := expression; ast.IsCallExpression(node) || ast.IsPropertyAccessExpression(node); node = node.Expression() {
			if ast.IsPropertyAccessExpression(node) && node.Expression().Virtual {
				p.etsStateRoot = strings.Replace(node.Expression().Text(), "Instance", "", 1)
				virtualType = p.etsStateRoot + "Attribute"
				break
			}
		}
	}
	if virtualType == "" {
		return original, stateProperty
	}
	typ := p.finishEtsVirtualNode(p.factory.NewTypeReferenceNode(p.etsVirtualIdentifier(virtualType, pos), nil), pos)
	return p.newNodeList(core.NewTextRange(p.nodePos(), p.nodePos()), p.nodeSliceArena.NewSlice1(typ)), stateProperty
}

func (p *Parser) parseArkUIArgumentList(expression *ast.Node) *ast.NodeList {
	if !p.arkUICallback {
		return p.parseArgumentList()
	}
	callback := p.arkUICallback
	defer func() { p.arkUICallback = callback }()
	// OH parseArgumentExpression suppresses the first data argument except each.
	skipFirst := !ast.IsPropertyAccessExpression(expression) || expression.Name().Text() != "each"
	index := 0
	p.parseExpected(ast.KindOpenParenToken)
	result := p.parseDelimitedList(PCArgumentExpressions, func(p *Parser) *ast.Node {
		p.arkUICallback = !skipFirst || index > 0
		index++
		return p.parseArgumentExpression()
	})
	p.parseExpected(ast.KindCloseParenToken)
	return result
}

func hasArkUIBody(node *ast.Node) bool {
	for ast.IsCallExpression(node) || ast.IsPropertyAccessExpression(node) {
		if ast.IsCallExpression(node) && node.AsCallExpression().EtsBody != nil {
			return true
		}
		node = node.Expression()
	}
	return false
}
