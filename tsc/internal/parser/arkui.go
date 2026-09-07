package parser

import (
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
)

// struct is contextual: existing TypeScript identifiers remain legal in ETS.
func (p *Parser) isStructDeclaration() bool {
	return p.scriptKind == core.ScriptKindETS && p.token == ast.KindIdentifier &&
		p.scanner.TokenValue() == "struct" && p.lookAhead((*Parser).nextTokenIsIdentifierOnSameLine)
}

func (p *Parser) parseArkUIFunctionBody(flags ParseFlags, message *diagnostics.Message, modifiers *ast.ModifierList, build bool) *ast.Node {
	savedUI, savedStyles, savedExpression, savedCallback := p.arkUI, p.arkUIStyles, p.arkUIExpression, p.arkUICallback
	p.arkUI = p.scriptKind == core.ScriptKindETS && (build || ast.HasArkUIDecorator(modifiers, "Builder", "LocalBuilder"))
	p.arkUIStyles = p.scriptKind == core.ScriptKindETS && ast.HasArkUIDecorator(modifiers, "Styles", "Extend", "AnimatableExtend")
	p.arkUIExpression, p.arkUICallback = false, false
	defer func() {
		p.arkUI, p.arkUIStyles, p.arkUIExpression, p.arkUICallback = savedUI, savedStyles, savedExpression, savedCallback
	}()
	return p.parseFunctionBlockOrSemicolon(flags, message)
}

func isArkUIComponentName(node *ast.Node) bool {
	if ast.IsExpressionWithTypeArguments(node) {
		return isArkUIComponentName(node.Expression())
	}
	if ast.IsPropertyAccessExpression(node) {
		node = node.Name()
	}
	if !ast.IsIdentifier(node) {
		return false
	}
	name := node.Text()
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

func isArkUIIteration(node *ast.Node) bool {
	if ast.IsIdentifier(node) {
		return node.Text() == "ForEach" || node.Text() == "LazyForEach"
	}
	if ast.IsPropertyAccessExpression(node) && (node.Name().Text() == "each" || node.Name().Text() == "template") {
		root := node.Expression()
		for ast.IsCallExpression(root) || ast.IsPropertyAccessExpression(root) {
			root = root.Expression()
		}
		return ast.IsIdentifier(root) && root.Text() == "Repeat"
	}
	return false
}

func (p *Parser) parseArkUIArgumentList(expression *ast.Node) *ast.NodeList {
	if !p.arkUICallback {
		return p.parseArgumentList()
	}
	callback := p.arkUICallback
	defer func() { p.arkUICallback = callback }()
	skipFirst := ast.IsIdentifier(expression)
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
