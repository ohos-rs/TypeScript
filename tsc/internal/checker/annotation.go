package checker

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/evaluator"
	"github.com/microsoft/TypeScript/tsc/internal/jsnum"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

func isAnnotationSymbol(symbol *ast.Symbol) bool {
	return symbol != nil && symbol.Flags&ast.SymbolFlagsAnnotation != 0
}

type AnnotationLinks struct {
	sourceRetention core.Tristate
	checked         bool
}

func isRetentionAnnotationDeclaration(node *ast.Node) bool {
	return node.Name().Text() == "Retention" && strings.EqualFold(filepath.Base(ast.GetSourceFileOfNode(node).FileName()), "@arkts.lang.d.ets")
}

// OH hasSourceRetentionPolicy examines the literal syntax (not arbitrary
// constant folding). In particular, a quoted property name is not `policy`.
func (c *Checker) hasSourceRetentionPolicy(node *ast.Node) bool {
	if isRetentionAnnotationDeclaration(node) {
		return false
	}
	for _, modifier := range node.ModifierNodes() {
		if !ast.IsDecorator(modifier) {
			continue
		}
		declaration := c.annotationForDecorator(modifier)
		if declaration == nil || !isRetentionAnnotationDeclaration(declaration) {
			continue
		}
		expression := modifier.Expression()
		if !ast.IsCallExpression(expression) || len(expression.Arguments()) != 1 || !ast.IsObjectLiteralExpression(expression.Arguments()[0]) {
			continue
		}
		for _, prop := range expression.Arguments()[0].AsObjectLiteralExpression().Properties.Nodes {
			if !ast.IsPropertyAssignment(prop) || scanner.GetTextOfNode(prop.Name()) != "policy" {
				continue
			}
			value := prop.Initializer()
			if ast.IsStringLiteral(value) {
				return value.Text() == "source"
			}
			if !ast.IsPropertyAccessExpression(value) {
				continue
			}
			symbol := c.resolveEntityName(value, ast.SymbolFlagsValue, true, false, nil)
			if symbol != nil && symbol.Flags&ast.SymbolFlagsEnumMember != 0 && ast.IsEnumConst(symbol.ValueDeclaration.Parent) && c.GetConstantValue(symbol.ValueDeclaration) == "source" {
				return true
			}
		}
	}
	return false
}

func (c *Checker) isSourceRetentionAnnotationDeclaration(node *ast.Node) bool {
	links := c.annotationLinks.Get(node)
	if links.sourceRetention == core.TSUnknown {
		links.sourceRetention = core.TSFalse
		// ets2bundle api_check_utils::isSourceRetentionDeclarationValid is
		// installed by its compiler host. SDK annotations need no Retention
		// decorator; identity is declaration name plus normalized file suffix.
		name := node.Name().Text()
		sdkSourceAnnotation := (name == "Available" || name == "SuppressWarnings") &&
			strings.HasSuffix(tspath.NormalizePath(ast.GetSourceFileOfNode(node).FileName()), "@ohos.annotation.d.ets")
		if sdkSourceAnnotation || c.hasSourceRetentionPolicy(node) {
			links.sourceRetention = core.TSTrue
		}
	}
	return links.sourceRetention == core.TSTrue
}

// OH checker.ts::checkAnnotationDeclaration. Class binding supplies member
// tables; annotation grammar and checking must not use ordinary class rules.
func (c *Checker) checkAnnotationDeclaration(node *ast.Node) {
	if !ast.IsSourceFile(node.Parent) {
		c.grammarErrorOnNode(node, diagnostics.Annotation_must_be_defined_at_top_level_only)
	}
	c.checkGrammarModifiers(node)
	c.checkAnnotationJsHar(node)
	if ast.HasSyntacticModifier(node, ast.ModifierFlagsDefault) {
		c.error(node, diagnostics.Annotation_cannot_be_exported_as_default)
	}
	for _, modifier := range node.ModifierNodes() {
		if ast.IsDecorator(modifier) {
			if declaration := c.annotationForDecorator(modifier); declaration != nil {
				c.checkAnnotationUse(modifier, declaration)
			} else {
				c.grammarErrorOnNode(modifier, diagnostics.Decorators_are_not_valid_here)
			}
		}
	}
	c.checkCollisionsForDeclarationName(node, node.Name())
	c.checkExportsOnMergedDeclarations(node)
	symbol := c.getSymbolOfDeclaration(node)
	t, staticType := c.getDeclaredTypeOfSymbol(symbol), c.getTypeOfSymbol(symbol)
	c.checkObjectTypeForDuplicateDeclarations(node, true)
	c.checkIndexConstraints(t, symbol, false)
	c.checkIndexConstraints(staticType, symbol, true)
	c.checkPropertyInitialization(node)
	c.checkSourceElements(node.Members())
}

func (c *Checker) annotationForDecorator(node *ast.Node) *ast.Node {
	if ast.GetSourceFileOfNode(node).ScriptKind != core.ScriptKindETS {
		return nil
	}
	expression := node.Expression()
	if ast.IsCallExpression(expression) {
		expression = expression.Expression()
	}
	if !ast.IsIdentifier(expression) && !ast.IsPropertyAccessExpression(expression) {
		return nil
	}
	symbol := c.resolveEntityName(expression, ast.SymbolFlagsValue, true, false, nil)
	if isAnnotationSymbol(symbol) {
		return symbol.ValueDeclaration
	}
	return nil
}

// OH resolveAnnotation/checkAnnotation: validate the parameter expression before
// its placement. Annotation uses never run JS decorator-call checking.
func (c *Checker) checkAnnotationUse(node, declaration *ast.Node) {
	links := c.annotationLinks.Get(node)
	if links.checked {
		return
	}
	links.checked = true
	if !c.isSourceRetentionAnnotationDeclaration(declaration) {
		c.checkAnnotationJsHar(node)
	}
	expression := node.Expression()
	// OH resolveAnnotation checks the reference for bare and zero-argument
	// annotations too. Merely resolving its symbol misses temporal dead zones.
	if !ast.IsCallExpression(expression) {
		c.checkExpression(expression)
	} else if len(expression.Arguments()) == 0 {
		c.checkExpression(expression.Expression())
	}
	var argument *ast.Node
	if ast.IsCallExpression(expression) {
		arguments := expression.Arguments()
		if len(arguments) > 0 {
			if len(arguments) != 1 || !ast.IsObjectLiteralExpression(arguments[0]) {
				c.error(node, diagnostics.Only_an_object_literal_have_to_be_provided_as_annotation_parameters_list_got_Colon_0, scanner.GetTextOfNode(arguments[0]))
				return
			}
			argument = arguments[0]
		}
	}
	if argument == nil || len(argument.AsObjectLiteralExpression().Properties.Nodes) == 0 {
		for _, member := range declaration.Members() {
			if member.Initializer() == nil {
				c.error(node, diagnostics.When_annotation_0_is_applied_all_fields_without_default_values_must_be_provided, scanner.GetTextOfNode(expression))
				return
			}
		}
	} else {
		for _, prop := range argument.AsObjectLiteralExpression().Properties.Nodes {
			if !ast.IsPropertyAssignment(prop) {
				c.error(node, diagnostics.Only_an_object_literal_have_to_be_provided_as_annotation_parameters_list_got_Colon_0, scanner.GetTextOfNode(argument))
				return
			}
			if c.evaluateAnnotationConstant(prop.Initializer(), make(map[*ast.Node]bool)) == nil {
				c.error(node, diagnostics.All_members_of_object_literal_which_is_provided_as_annotation_parameters_list_have_to_be_constant_expressions_got_Colon_0, scanner.GetTextOfNode(prop.Initializer()))
				return
			}
		}
		if c.isErrorType(c.checkExpression(expression)) {
			return
		}
	}
	parent := node.Parent
	if c.isSourceRetentionAnnotationDeclaration(declaration) {
		if !isSourceRetentionAnnotationTarget(parent) {
			c.error(node, diagnostics.X_0_annotation_are_not_valid_here_got_Colon_1, scanner.GetTextOfNode(declaration.Name()), scanner.GetTextOfNode(parent))
			return
		}
	} else if isRetentionAnnotationDeclaration(declaration) {
		if !ast.IsAnnotationDeclaration(parent) {
			c.error(node, diagnostics.X_0_should_only_be_applied_to_annotation_declarations, "@Retention")
			return
		}
	} else {
		class := func(n *ast.Node) bool {
			return ast.IsClassDeclaration(n) && !ast.IsAnnotationDeclaration(n) && !ast.IsStructDeclaration(n)
		}
		if !class(parent) && !(ast.IsMethodDeclaration(parent) && class(parent.Parent)) {
			switch parent.Kind {
			case ast.KindConstructor:
				c.error(node, diagnostics.Annotation_cannot_be_applied_for_constructor_got_Colon_0, scanner.GetTextOfNode(parent))
			case ast.KindGetAccessor, ast.KindSetAccessor:
				c.error(node, diagnostics.Annotation_cannot_be_applied_for_getter_or_setter_got_Colon_0, scanner.GetTextOfNode(parent))
			default:
				if ast.IsAnnotationDeclaration(parent) {
					c.error(node, diagnostics.Annotation_cannot_be_applied_for_annotation_declaration)
				} else {
					c.error(node, diagnostics.Annotation_have_to_be_applied_for_classes_or_methods_in_classes_only_got_Colon_0, scanner.GetTextOfNode(parent))
				}
			}
			return
		}
		if ast.HasSyntacticModifier(parent, ast.ModifierFlagsAbstract) || ast.IsMethodDeclaration(parent) && ast.HasSyntacticModifier(parent.Parent, ast.ModifierFlagsAbstract) {
			c.error(node, diagnostics.Annotation_have_to_be_applied_only_for_non_abstract_class_declarations_and_method_declarations_in_non_abstract_classes_got_Colon_0, scanner.GetTextOfNode(parent))
			return
		}
	}
	for _, previous := range parent.ModifierNodes() {
		if previous == node {
			break
		}
		if ast.IsDecorator(previous) && c.annotationForDecorator(previous) == declaration {
			c.error(node, diagnostics.Repeatable_annotation_are_not_supported_got_Colon_0, scanner.GetTextOfNode(node))
			return
		}
	}
	if c.isSourceRetentionAnnotationDeclaration(declaration) {
		c.checkSourceRetentionAnnotationContent(node, declaration)
	}
}

// OH inModuleRootPath intentionally compares normalized string prefixes. Do
// not replace it with a different filesystem containment or realpath policy.
func (c *Checker) checkAnnotationJsHar(node *ast.Node) {
	if c.compilerOptions.IsCompileJsHar == core.TSTrue && c.compilerOptions.ModuleRootPath != "" &&
		strings.HasPrefix(tspath.NormalizePath(ast.GetSourceFileOfNode(node).FileName()), tspath.NormalizePath(c.compilerOptions.ModuleRootPath)) {
		c.grammarErrorOnNode(node, diagnostics.Annotations_are_not_supported_in_Hars_compiled_to_JavaScript_files)
	}
}

// OH checkSourceRetentionAnnotation: source-retained annotations have a wider
// target set, but still exclude parameters, constructors and annotation fields.
func isSourceRetentionAnnotationTarget(node *ast.Node) bool {
	switch node.Kind {
	case ast.KindClassDeclaration, ast.KindFunctionDeclaration, ast.KindVariableStatement, ast.KindTypeAliasDeclaration, ast.KindInterfaceDeclaration, ast.KindEnumDeclaration, ast.KindModuleDeclaration:
		return true
	case ast.KindGetAccessor, ast.KindSetAccessor, ast.KindMethodDeclaration, ast.KindPropertyDeclaration:
		return !ast.IsAnnotationPropertyDeclaration(node) && (ast.IsClassDeclaration(node.Parent) && !ast.IsAnnotationDeclaration(node.Parent) || ast.IsInterfaceDeclaration(node.Parent))
	case ast.KindPropertySignature, ast.KindMethodSignature:
		return ast.IsInterfaceDeclaration(node.Parent)
	}
	return false
}

func (c *Checker) checkAnnotationPropertyDeclaration(node *ast.Node) {
	if node.Type() == nil && node.Initializer() == nil {
		c.error(node, diagnostics.An_annotation_property_must_have_a_type_or_Slashand_an_initializer)
	}
	c.checkVariableLikeDeclaration(node)
	t := c.getTypeOfNode(node)
	if !c.isAllowedAnnotationPropertyType(t) {
		c.error(node, diagnostics.A_type_of_annotation_property_have_to_be_number_boolean_string_const_enumeration_types_or_array_of_above_types_got_Colon_0, c.typeToString(t, nil))
	}
	if init := node.Initializer(); init != nil && c.evaluateAnnotationConstant(init, make(map[*ast.Node]bool)) == nil {
		c.error(node, diagnostics.Default_value_of_annotation_property_can_be_a_constant_expression_got_Colon_0, scanner.GetTextOfNode(init))
	}
}

func (c *Checker) isAllowedAnnotationPropertyType(t *Type) bool {
	if t == c.numberType || t == c.stringType || t == c.booleanType {
		return true
	}
	if element := c.getElementTypeOfArrayType(t); element != nil {
		return c.isAllowedAnnotationPropertyType(element)
	}
	symbol := t.symbol
	if symbol == nil || (symbol.Flags&ast.SymbolFlagsConstEnum == 0 && (symbol.Parent == nil || symbol.Parent.Flags&ast.SymbolFlagsConstEnum == 0)) {
		return false
	}
	// Preserve OH isAllowedAnnotationPropertyEnumType's i > 1 comparison.
	for _, decl := range symbol.Declarations {
		if ast.IsEnumDeclaration(decl) {
			members := decl.Members()
			for i := 2; i < len(members); i++ {
				if reflect.TypeOf(c.GetConstantValue(members[i-1])) != reflect.TypeOf(c.GetConstantValue(members[i])) {
					return false
				}
			}
		}
	}
	return true
}

// OH evaluateAnnotationPropertyConstantExpression, not the ordinary enum
// evaluator, which also accepts mixed number/string concatenation.
func (c *Checker) evaluateAnnotationConstant(node *ast.Node, active map[*ast.Node]bool) any {
	if active[node] {
		return nil
	}
	active[node] = true
	defer delete(active, node)
	switch node.Kind {
	case ast.KindTrueKeyword:
		return true
	case ast.KindFalseKeyword:
		return false
	case ast.KindNumericLiteral:
		return jsnum.FromString(node.Text())
	case ast.KindStringLiteral, ast.KindNoSubstitutionTemplateLiteral:
		return node.Text()
	case ast.KindParenthesizedExpression:
		return c.evaluateAnnotationConstant(node.Expression(), active)
	case ast.KindIdentifier:
		if node.Text() == "Infinity" || node.Text() == "NaN" {
			return jsnum.FromString(node.Text())
		}
		symbol := c.getExportSymbolOfValueSymbolIfExported(c.getResolvedSymbol(node))
		if symbol != nil && symbol.ValueDeclaration != nil && ast.IsVariableDeclaration(symbol.ValueDeclaration) && c.isVarConstLike(symbol.ValueDeclaration) {
			if init := symbol.ValueDeclaration.Initializer(); init != nil {
				return c.evaluateAnnotationConstant(init, active)
			}
		}
	case ast.KindPropertyAccessExpression:
		return c.GetConstantValue(node)
	case ast.KindArrayLiteralExpression:
		values := make([]any, 0, len(node.AsArrayLiteralExpression().Elements.Nodes))
		for _, element := range node.AsArrayLiteralExpression().Elements.Nodes {
			value := c.evaluateAnnotationConstant(element, active)
			if value == nil || len(values) > 0 && reflect.TypeOf(values[len(values)-1]) != reflect.TypeOf(value) {
				return nil
			}
			values = append(values, value)
		}
		return values
	case ast.KindPrefixUnaryExpression:
		expr := node.AsPrefixUnaryExpression()
		value := c.evaluateAnnotationConstant(expr.Operand, active)
		if value == nil {
			return nil
		}
		if expr.Operator == ast.KindExclamationToken && annotationScalar(value) {
			return !evaluator.IsTruthy(value)
		}
		if n, ok := value.(jsnum.Number); ok {
			switch expr.Operator {
			case ast.KindPlusToken:
				return n
			case ast.KindMinusToken:
				return -n
			case ast.KindTildeToken:
				return n.BitwiseNOT()
			}
		}
	case ast.KindBinaryExpression:
		expr := node.AsBinaryExpression()
		return annotationBinary(expr.OperatorToken.Kind, c.evaluateAnnotationConstant(expr.Left, active), c.evaluateAnnotationConstant(expr.Right, active))
	}
	return nil
}

func annotationScalar(value any) bool {
	switch value.(type) {
	case jsnum.Number, string, bool:
		return true
	}
	return false
}

func annotationBinary(operator ast.Kind, left, right any) any {
	if left == nil || right == nil {
		return nil
	}
	if annotationScalar(left) && annotationScalar(right) {
		if operator == ast.KindAmpersandAmpersandToken {
			if evaluator.IsTruthy(left) {
				return right
			}
			return left
		}
		if operator == ast.KindBarBarToken {
			if evaluator.IsTruthy(left) {
				return left
			}
			return right
		}
	}
	if reflect.TypeOf(left) == reflect.TypeOf(right) {
		if l, ok := left.([]any); ok {
			r := right.([]any)
			switch operator {
			case ast.KindAmpersandAmpersandToken:
				return right
			case ast.KindBarBarToken:
				return left
			case ast.KindLessThanToken, ast.KindLessThanEqualsToken, ast.KindGreaterThanToken, ast.KindGreaterThanEqualsToken:
				return annotationBinary(operator, annotationArrayString(l), annotationArrayString(r))
			}
		}
		equal := false
		if annotationScalar(left) {
			equal = left == right
		}
		switch operator {
		case ast.KindEqualsEqualsToken, ast.KindEqualsEqualsEqualsToken:
			return equal
		case ast.KindExclamationEqualsToken, ast.KindExclamationEqualsEqualsToken:
			return !equal
		}
	}
	if l, ok := left.(jsnum.Number); ok {
		if r, ok := right.(jsnum.Number); ok {
			switch operator {
			case ast.KindPlusToken:
				return l + r
			case ast.KindMinusToken:
				return l - r
			case ast.KindAsteriskToken:
				return l * r
			case ast.KindSlashToken:
				return l / r
			case ast.KindPercentToken:
				return l.Remainder(r)
			case ast.KindAsteriskAsteriskToken:
				return l.Exponentiate(r)
			case ast.KindBarToken:
				return l.BitwiseOR(r)
			case ast.KindAmpersandToken:
				return l.BitwiseAND(r)
			case ast.KindCaretToken:
				return l.BitwiseXOR(r)
			case ast.KindLessThanLessThanToken:
				return l.LeftShift(r)
			case ast.KindGreaterThanGreaterThanToken:
				return l.SignedRightShift(r)
			case ast.KindGreaterThanGreaterThanGreaterThanToken:
				return l.UnsignedRightShift(r)
			case ast.KindLessThanToken:
				return l < r
			case ast.KindLessThanEqualsToken:
				return l <= r
			case ast.KindGreaterThanToken:
				return l > r
			case ast.KindGreaterThanEqualsToken:
				return l >= r
			}
		}
	}
	if l, ok := left.(string); ok {
		if r, ok := right.(string); ok {
			order := slices.Compare(utf16.Encode([]rune(l)), utf16.Encode([]rune(r)))
			switch operator {
			case ast.KindPlusToken:
				return l + r
			case ast.KindLessThanToken:
				return order < 0
			case ast.KindLessThanEqualsToken:
				return order <= 0
			case ast.KindGreaterThanToken:
				return order > 0
			case ast.KindGreaterThanEqualsToken:
				return order >= 0
			}
		}
	}
	if l, ok := left.(bool); ok {
		if r, ok := right.(bool); ok {
			switch operator {
			case ast.KindLessThanToken:
				return !l && r
			case ast.KindLessThanEqualsToken:
				return !l || r
			case ast.KindGreaterThanToken:
				return l && !r
			case ast.KindGreaterThanEqualsToken:
				return l || !r
			}
		}
	}
	return nil
}

// JS relational comparison ToPrimitive for evaluated annotation arrays.
func annotationArrayString(values []any) string {
	parts := make([]string, len(values))
	for i, value := range values {
		if nested, ok := value.([]any); ok {
			parts[i] = annotationArrayString(nested)
		} else {
			parts[i] = evaluator.AnyToString(value)
		}
	}
	return strings.Join(parts, ",")
}
