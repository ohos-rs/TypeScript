package checker

import "github.com/microsoft/TypeScript/tsc/internal/ast"

// ArkUI constructs a struct with an optional property bag. Use a real signature
// so contextual typing, excess property checks and signature help work normally.
func (c *Checker) getArkUIStructSignature(t *Type) *Signature {
	if t.symbol == nil || !ast.IsStructDeclaration(t.symbol.ValueDeclaration) {
		return nil
	}
	instance := c.getDeclaredTypeOfSymbol(t.symbol)
	typeParameters := instance.AsInterfaceType().LocalTypeParameters()
	if common := c.getArkUIAttributeType(t.symbol.ValueDeclaration, "CommonAttribute"); common != nil {
		instance = c.getIntersectionType([]*Type{instance, common})
	}
	properties := make(ast.SymbolTable)
	required := false
	for _, member := range t.symbol.ValueDeclaration.Members() {
		if !ast.IsPropertyDeclaration(member) || ast.IsStatic(member) || member.ModifierFlags()&ast.ModifierFlagsPrivate != 0 {
			continue
		}
		symbol := c.getSymbolOfDeclaration(member)
		property := c.newProperty(symbol.Name, c.getTypeOfSymbol(symbol))
		property.Declarations = symbol.Declarations
		property.ValueDeclaration = symbol.ValueDeclaration
		if ast.HasArkUIDecorator(member.Modifiers(), "Require") {
			required = true
		} else {
			property.Flags |= ast.SymbolFlagsOptional
		}
		properties[symbol.Name] = property
	}
	var propsSymbol *ast.Symbol
	if len(typeParameters) != 0 {
		// Give the synthesized property bag its own declaration and instantiation
		// scope. A symbol-less anonymous type would keep T unsubstituted.
		declaration := c.factory.NewTypeLiteralNode(c.factory.NewNodeList(nil))
		declaration.Parent = t.symbol.ValueDeclaration
		propsSymbol = c.newSymbol(ast.SymbolFlagsTypeLiteral, ast.InternalSymbolNameType)
		propsSymbol.Declarations = []*ast.Node{declaration}
		c.typeNodeLinks.Get(declaration).outerTypeParameters = typeParameters
	}
	props := c.newAnonymousType(propsSymbol, properties, nil, nil, nil)
	parameter := c.newParameter("props", props)
	minArguments := 0
	if required {
		minArguments = 1
	}
	return c.newSignature(SignatureFlagsNone, nil, typeParameters, nil, []*ast.Symbol{parameter}, instance, nil, minArguments)
}

func (c *Checker) getArkUIAttributeType(location *ast.Node, name string) *Type {
	symbol := c.resolveName(location, name, ast.SymbolFlagsType, nil, true, false)
	if symbol == nil {
		return nil
	}
	return c.getDeclaredTypeOfSymbol(c.resolveSymbol(symbol))
}

func (c *Checker) getArkUIImplicitReceiverType(node *ast.Node) *Type {
	name := "CommonAttribute"
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if ast.IsCallExpression(parent) && ast.IsPropertyAccessExpression(parent.Expression()) && parent.Expression().Name().Text() == "stateStyles" {
			root := parent.Expression().Expression()
			for ast.IsCallExpression(root) || ast.IsPropertyAccessExpression(root) {
				root = root.Expression()
			}
			if ast.IsIdentifier(root) {
				name = root.Text() + "Attribute"
			}
			break
		}
		if !ast.IsFunctionDeclaration(parent) && !ast.IsMethodDeclaration(parent) {
			continue
		}
		for _, modifier := range parent.ModifierNodes() {
			if ast.IsDecorator(modifier) && ast.IsCallExpression(modifier.Expression()) {
				expr := modifier.Expression()
				if ast.IsIdentifier(expr.Expression()) && (expr.Expression().Text() == "Extend" || expr.Expression().Text() == "AnimatableExtend") && len(expr.Arguments()) == 1 && ast.IsIdentifier(expr.Arguments()[0]) {
					name = expr.Arguments()[0].Text() + "Attribute"
				}
			}
		}
		break
	}
	if t := c.getArkUIAttributeType(node, name); t != nil {
		return t
	}
	// SDK declarations may be absent while editing a standalone source file.
	return c.anyType
}

func (c *Checker) getArkUIBindingSymbol(node *ast.Node) *ast.Symbol {
	// User declarations such as $r and $rawfile take precedence over bindings.
	if symbol := c.resolveName(node, node.Text(), ast.SymbolFlagsValue|ast.SymbolFlagsAlias, nil, false, false); symbol != nil {
		return symbol
	}
	name := node.Text()
	if name == "$$this" {
		if class := ast.GetContainingClass(node); class != nil {
			return class.Symbol()
		}
	}
	if len(name) < 2 || name[0] != '$' {
		return nil
	}
	if name[1] == '$' {
		return c.resolveName(node, name[2:], ast.SymbolFlagsValue|ast.SymbolFlagsAlias, nil, true, false)
	}
	if class := ast.GetContainingClass(node); ast.IsStructDeclaration(class) {
		return c.getPropertyOfType(c.getDeclaredTypeOfSymbol(class.Symbol()), name[1:])
	}
	return nil
}

// Styles and Extend declarations introduce fluent attributes in render bodies.
// Resolve the original symbol so references and parameter checking stay useful.
func (c *Checker) getArkUIStyleProperty(node, left, right *ast.Node, receiver *Type) *ast.Symbol {
	root := left
	for ast.IsCallExpression(root) || ast.IsPropertyAccessExpression(root) {
		if ast.IsEtsComponentExpression(root) {
			break
		}
		root = root.Expression()
	}
	if !ast.IsEtsComponentExpression(root) || !ast.IsIdentifier(root.Expression()) {
		return nil
	}
	var symbol *ast.Symbol
	if class := ast.GetContainingClass(node); class != nil && ast.IsStructDeclaration(class) {
		symbol = c.getPropertyOfType(c.getDeclaredTypeOfSymbol(class.Symbol()), right.Text())
	}
	if symbol == nil {
		symbol = c.resolveName(right, right.Text(), ast.SymbolFlagsValue|ast.SymbolFlagsAlias, nil, true, false)
		if symbol != nil {
			symbol = c.resolveSymbol(symbol)
		}
	}
	if symbol == nil || symbol.ValueDeclaration == nil {
		return nil
	}
	declaration := symbol.ValueDeclaration
	allowed := ast.HasArkUIDecorator(declaration.Modifiers(), "Styles")
	for _, modifier := range declaration.ModifierNodes() {
		if !ast.IsDecorator(modifier) || !ast.IsCallExpression(modifier.Expression()) {
			continue
		}
		expr := modifier.Expression()
		if ast.IsIdentifier(expr.Expression()) && (expr.Expression().Text() == "Extend" || expr.Expression().Text() == "AnimatableExtend") && len(expr.Arguments()) == 1 && ast.IsIdentifier(expr.Arguments()[0]) {
			allowed = expr.Arguments()[0].Text() == root.Expression().Text()
		}
	}
	if !allowed {
		return nil
	}
	var signatures []*Signature
	for _, signature := range c.getSignaturesOfType(c.getTypeOfSymbol(symbol), SignatureKindCall) {
		signatures = append(signatures, c.newSignature(signature.flags, signature.declaration, signature.typeParameters, nil, signature.parameters, receiver, nil, int(signature.minArgumentCount)))
	}
	property := c.newProperty(right.Text(), c.newAnonymousType(nil, nil, signatures, nil, nil))
	property.Declarations, property.ValueDeclaration = symbol.Declarations, symbol.ValueDeclaration
	return property
}
