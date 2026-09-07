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

func IsEtsComponentExpression(node *Node) bool {
	return node != nil && IsCallExpression(node) && node.Flags&NodeFlagsEtsComponent != 0
}

func ContainsArkUISyntax(file *SourceFile) bool {
	if file.ScriptKind != core.ScriptKindETS {
		return false
	}
	var visit Visitor
	visit = func(node *Node) bool {
		return IsStructDeclaration(node) || IsEtsComponentExpression(node) ||
			IsArkUICompilerDecorator(node) || node.ForEachChild(visit)
	}
	return visit(file.AsNode())
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

// ArkUI annotations are interpreted by the ArkUI compiler, not as JavaScript
// decorator calls. Keep this exception local to ETS and the supported targets.
func IsArkUICompilerDecorator(decorator *Node) bool {
	if !IsDecorator(decorator) || GetSourceFileOfNode(decorator).ScriptKind != core.ScriptKindETS {
		return false
	}
	expr := decorator.Expression()
	if IsCallExpression(expr) {
		expr = expr.Expression()
	}
	if !IsIdentifier(expr) {
		return false
	}
	name := expr.Text()
	parent := decorator.Parent
	switch parent.Kind {
	case KindClassDeclaration:
		if IsStructDeclaration(parent) {
			return slices.Contains([]string{"Entry", "Component", "ComponentV2", "Reusable", "CustomDialog", "Preview"}, name)
		}
		return slices.Contains([]string{"Observed", "ObservedV2", "Sendable"}, name)
	case KindFunctionDeclaration:
		return slices.Contains([]string{"Builder", "Styles", "Extend", "AnimatableExtend", "Concurrent"}, name)
	case KindMethodDeclaration:
		return slices.Contains([]string{"Builder", "LocalBuilder", "Styles", "Monitor", "Computed"}, name)
	case KindPropertyDeclaration:
		return slices.Contains([]string{"State", "Prop", "Link", "Provide", "Consume", "ObjectLink", "StorageLink", "StorageProp", "LocalStorageLink", "LocalStorageProp", "BuilderParam", "Watch", "Require", "Track", "Trace", "Local", "Param", "Once", "Event", "Provider", "Consumer"}, name)
	}
	return false
}
