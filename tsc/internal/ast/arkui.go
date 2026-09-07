package ast

import (
	"slices"

	"github.com/microsoft/TypeScript/tsc/internal/core"
)

// IsStructDeclaration identifies an ArkUI struct. Its class representation lets
// binding, symbol lookup and member checking share TypeScript's implementation.
func IsStructDeclaration(node *Node) bool {
	return node != nil && IsClassDeclaration(node) && node.Flags&NodeFlagsStruct != 0
}

// OH binder.ts::bindAnnotationDeclaration uses class binding plus an annotation
// identity. Share the class/member storage, never the ordinary class semantics.
func IsAnnotationDeclaration(node *Node) bool {
	return node != nil && IsClassDeclaration(node) && node.Flags&NodeFlagsAnnotation != 0
}

func IsAnnotationPropertyDeclaration(node *Node) bool {
	return node != nil && IsPropertyDeclaration(node) && node.Flags&NodeFlagsAnnotation != 0
}

func IsEtsComponentExpression(node *Node) bool {
	return node != nil && IsCallExpression(node) && node.Flags&NodeFlagsEtsComponent != 0
}

func HasArkUIDecorator(modifiers *ModifierList, names ...string) bool {
	if modifiers != nil {
		for _, node := range modifiers.Nodes {
			if IsDecorator(node) {
				expr := node.AsDecorator().Expression
				if IsCallExpression(expr) {
					expr = expr.Expression()
				}
				if IsIdentifier(expr) && slices.Contains(names, expr.Text()) {
					return true
				}
			}
		}
	}
	return false
}

// OH hasEtsBuilderDecoratorNames/hasEtsStylesDecoratorNames inspect bare
// identifiers. A call or a qualified name must not enable their DSL context.
func HasArkUIBareDecorator(modifiers *ModifierList, names ...string) bool {
	if modifiers != nil {
		for _, modifier := range modifiers.Nodes {
			if IsDecorator(modifier) && IsIdentifier(modifier.Expression()) && slices.Contains(names, modifier.Expression().Text()) {
				return true
			}
		}
	}
	return false
}

// OH ohApi.ts::isArkTsDecorator. This controls valid declaration targets,
// never suppresses normal SDK decorator name/signature checking.
func HasEtsDecorator(node *Node, options core.EtsOptions) bool {
	for _, modifier := range node.ModifierNodes() {
		if !IsDecorator(modifier) {
			continue
		}
		expression := modifier.Expression()
		if IsCallExpression(expression) && IsIdentifier(expression.Expression()) && options.Extend.Decorator.Contains(expression.Expression().Text()) {
			return true
		}
		if IsIdentifier(expression) {
			name := expression.Text()
			if options.Render.Decorator.Contains(name) {
				return true
			}
			if style, ok := options.Styles.Decorator.Get(); ok && style == name {
				return true
			}
			if concurrent, ok := options.Concurrent.Decorator.Get(); ok && concurrent == name {
				return true
			}
		}
	}
	return false
}

// IsEtsFunctionDecorator mirrors ohApi.ts::isEtsFunctionDecorators. The
// concurrent decorator is intentionally excluded because OH only extends
// function-decorator call semantics for render, Styles, and Extend.
func IsEtsFunctionDecorator(decorator *Node, options core.EtsOptions) bool {
	if !IsDecorator(decorator) {
		return false
	}
	expression := decorator.Expression()
	if IsCallExpression(expression) {
		expression = expression.Expression()
	}
	if !IsIdentifier(expression) {
		return false
	}
	name := expression.Text()
	if options.Render.Decorator.Contains(name) || options.Extend.Decorator.Contains(name) {
		return true
	}
	styles, present := options.Styles.Decorator.Get()
	return present && styles == name
}

func HasEtsStylesDecorator(node *Node, options core.EtsOptions) bool {
	name, present := options.Styles.Decorator.Get()
	return present && HasArkUIBareDecorator(node.Modifiers(), name)
}

func IsEtsBuilder(node *Node, options core.EtsOptions) bool {
	if options.Render.Decorator.IsZero() {
		return HasArkUIBareDecorator(node.Modifiers(), "Builder", "LocalBuilder")
	}
	for name := range options.Render.Decorator.Values() {
		if HasArkUIBareDecorator(node.Modifiers(), name) {
			return true
		}
	}
	return false
}

// OH isSendableFunctionOrType requires exactly one bare decorator in ETS.
func IsSendableFunctionOrType(node *Node) bool {
	if !(IsFunctionDeclaration(node) || IsTypeAliasDeclaration(node)) || GetSourceFileOfNode(node).ScriptKind != core.ScriptKindETS {
		return false
	}
	count, sendable := 0, false
	for _, modifier := range node.ModifierNodes() {
		if IsDecorator(modifier) {
			count++
			sendable = IsIdentifier(modifier.Expression()) && modifier.Expression().Text() == "Sendable"
		}
	}
	return count == 1 && sendable
}
