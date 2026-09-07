package parser

import (
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
)

func (p *Parser) isAnnotationDeclaration() bool {
	return p.scriptKind == core.ScriptKindETS && p.opts.EtsAnnotationsEnable && p.token == ast.KindAtToken && p.lookAhead(func(p *Parser) bool {
		return p.nextToken() == ast.KindInterfaceKeyword
	})
}

// OH parser.ts::parseAnnotationDeclaration. In particular, this is neither a
// TS interface nor a decorator named "interface". Whitespace is diagnosed,
// rather than making the parser fall back to decorator syntax.
func (p *Parser) parseAnnotationDeclaration(pos int, jsdoc jsdocScannerInfo, modifiers *ast.ModifierList) *ast.Node {
	at := p.scanner.TokenStart()
	p.parseExpected(ast.KindAtToken)
	keyword := p.scanner.TokenStart()
	p.parseExpected(ast.KindInterfaceKeyword)
	if keyword-at > 1 {
		p.parseErrorAt(at+1, keyword, diagnostics.In_annotation_declaration_any_symbols_between_and_interface_are_forbidden)
	}
	name := p.createIdentifier(p.isBindingIdentifier())
	var members *ast.NodeList
	if p.parseExpected(ast.KindOpenBraceToken) {
		members = p.parseList(PCAnnotationMembers, (*Parser).parseAnnotationElement)
		p.parseExpected(ast.KindCloseBraceToken)
	} else {
		members = p.createMissingList()
	}
	result := p.factory.NewClassDeclaration(modifiers, name, nil, nil, members)
	result.Flags |= ast.NodeFlagsAnnotation
	p.finishNode(result, pos)
	p.withJSDoc(result, jsdoc)
	return result
}

// OH parser.ts::isAnnotationMemberStart: identifier property names are followed
// by a colon, initializer or semicolon boundary; no keywords/modifiers/methods.
func (p *Parser) scanAnnotationMemberStart() bool {
	if !tokenIsIdentifierOrKeyword(p.token) || ast.IsKeyword(p.token) {
		return false
	}
	p.nextToken()
	return p.token == ast.KindColonToken || p.token == ast.KindEqualsToken || p.canParseSemicolon()
}

func (p *Parser) parseAnnotationElement() *ast.Node {
	pos, jsdoc := p.nodePos(), p.jsdocScannerInfo()
	name := p.parsePropertyName()
	typeNode := p.parseTypeAnnotation()
	initializer := doInContext(p, ast.NodeFlagsYieldContext|ast.NodeFlagsAwaitContext|ast.NodeFlagsDisallowInContext, false, (*Parser).parseInitializer)
	p.parseSemicolonAfterPropertyName(name, typeNode, initializer)
	node := p.factory.NewPropertyDeclaration(nil, name, nil, typeNode, initializer)
	node.Flags |= ast.NodeFlagsAnnotation
	p.finishNode(node, pos)
	p.withJSDoc(node, jsdoc)
	return node
}
