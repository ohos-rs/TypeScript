package checker

import (
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
)

// OH checker.ts::checkAllCodePathsInNonVoidFunctionReturnOrThrowDiagnostics.
// Check the return symbol against the configured type; do not replace it with
// CommonAttribute/any or synthesize a return expression.
func (c *Checker) checkEtsStyleReturnType(node *ast.Node, result *Type) bool {
	if result == nil {
		return false
	}
	options := c.compilerOptions.Ets
	var expected string
	var present bool
	var message *diagnostics.Message
	if ast.IsFunctionDeclaration(node) {
		last := ""
		for _, decorator := range node.ModifierNodes() {
			if !ast.IsDecorator(decorator) || !ast.IsCallExpression(decorator.Expression()) {
				continue
			}
			call := decorator.Expression()
			if !ast.IsIdentifier(call.Expression()) || len(call.Arguments()) == 0 || !ast.IsIdentifier(call.Arguments()[0]) {
				continue
			}
			name := call.Expression().Text()
			if options.Extend.Decorator.Contains(name) || options.Extend.Decorator.IsZero() && strings.Contains("Extend", name) {
				last = call.Arguments()[0].Text()
			}
		}
		if last != "" {
			message = diagnostics.Should_not_add_return_type_to_the_function_that_is_annotated_by_Extend
			for component := range options.Extend.Components.Values() {
				if component.Name == last {
					expected, present = component.Type, true
				}
			}
		}
	}
	if message == nil && (ast.IsFunctionDeclaration(node) || ast.IsMethodDeclaration(node)) && ast.HasArkUIBareDecorator(node.Modifiers(), options.Styles.Decorator.Or("Styles")) {
		message = diagnostics.Should_not_add_return_type_to_the_function_that_is_annotated_by_Styles
		if component, ok := options.Styles.Component.Get(); ok {
			expected, present = component.Type, true
		}
	}
	if message == nil {
		return false
	}
	if !(result.symbol == nil && !present || result.symbol != nil && present && result.symbol.Name == expected) {
		c.error(node.Type(), message)
	}
	return true
}
