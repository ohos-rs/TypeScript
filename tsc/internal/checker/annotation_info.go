package checker

import "github.com/microsoft/TypeScript/tsc/internal/ast"

// AnnotationInfo exposes type/checker facts, never generated expressions.
// The consumer owns OH ohApi.ts::transformAnnotation code generation.
type AnnotationInfo struct {
	Declaration     *ast.Node
	SourceRetention bool
	Properties      []AnnotationPropertyInfo
}

type AnnotationPropertyInfo struct {
	Declaration     *ast.Node
	Type            *Type
	Initializer     any
	Argument        any
	ArrayDepth      int
	ElementType     *Type
	EnumDeclaration *ast.Node
	EnumFirstValue  any
}

// GetAnnotationInfo supplies the inputs to OH's
// getAnnotationPropertyEvaluatedInitializer/getAnnotationObjectLiteralEvaluatedProps
// and enum-default handling. Property order is declaration order, not call-site
// object order. Nil constants mean absent/invalid, not a JavaScript null value
// (null is forbidden by the annotation constant-expression grammar).
func (c *Checker) GetAnnotationInfo(node *ast.Node) *AnnotationInfo {
	if node == nil {
		return nil
	}
	declaration := node
	var arguments map[string]any
	if ast.IsDecorator(node) {
		declaration = c.annotationForDecorator(node)
		if declaration == nil {
			return nil
		}
		c.checkAnnotationUse(node, declaration)
		if expression := node.Expression(); ast.IsCallExpression(expression) && len(expression.Arguments()) != 0 {
			if len(expression.Arguments()) != 1 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
				return nil
			}
			arguments = make(map[string]any)
			for _, property := range expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes {
				if !ast.IsPropertyAssignment(property) {
					return nil
				}
				value := c.evaluateAnnotationConstant(property.Initializer(), make(map[*ast.Node]bool))
				if value == nil {
					return nil
				}
				name, ok := ast.TryGetTextOfPropertyName(property.Name())
				if !ok {
					return nil
				}
				arguments[name] = value
			}
		}
	}
	if !ast.IsAnnotationDeclaration(declaration) {
		return nil
	}
	info := &AnnotationInfo{Declaration: declaration, SourceRetention: c.isSourceRetentionAnnotationDeclaration(declaration)}
	for _, member := range declaration.Members() {
		if !ast.IsAnnotationPropertyDeclaration(member) {
			return nil
		}
		typ := c.getTypeOfNode(member)
		if !c.isAllowedAnnotationPropertyType(typ) {
			return nil
		}
		property := AnnotationPropertyInfo{Declaration: member, Type: typ, ElementType: typ}
		if initializer := member.Initializer(); initializer != nil {
			property.Initializer = c.evaluateAnnotationConstant(initializer, make(map[*ast.Node]bool))
			if property.Initializer == nil {
				return nil
			}
		}
		name, ok := ast.TryGetTextOfPropertyName(member.Name())
		if !ok {
			return nil
		}
		property.Argument = arguments[name]
		for element := c.getElementTypeOfArrayType(property.ElementType); element != nil; element = c.getElementTypeOfArrayType(property.ElementType) {
			property.ArrayDepth++
			property.ElementType = element
		}
		if symbol := property.ElementType.symbol; symbol != nil && symbol.Flags&ast.SymbolFlagsConstEnum != 0 {
			for _, decl := range symbol.Declarations {
				if ast.IsEnumDeclaration(decl) {
					property.EnumDeclaration = decl
					if members := decl.Members(); len(members) != 0 {
						property.EnumFirstValue = c.GetConstantValue(members[0])
					}
					break
				}
			}
		}
		info.Properties = append(info.Properties, property)
	}
	return info
}
