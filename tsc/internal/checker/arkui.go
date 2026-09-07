package checker

import (
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

// OH checkPropertyAccessExpressionOrQualifiedName uses source-file locals,
// then enclosing struct Styles members. It returns the original symbol and
// signature; the parser's virtual type argument supplies fluent inference.
func (c *Checker) getArkUIStyleProperty(node, left, right *ast.Node) *ast.Symbol {
	root := etsComponentRoot(left)
	if root == nil {
		// OH getRootEtsComponentInnerCallExpressionNode/isInStateStylesObject.
		for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
			if ast.IsObjectLiteralExpression(ancestor) {
				if ancestor.Parent != nil && ast.IsPropertyAssignment(ancestor.Parent) {
					for call := ancestor.Parent; call != nil; call = call.Parent {
						if ast.IsCallExpression(call) {
							root = etsComponentRoot(call)
							if root != nil {
								break
							}
						}
					}
				}
				break
			}
		}
	}
	if root == nil {
		return nil
	}
	var symbol *ast.Symbol
	options := c.compilerOptions.Ets
	if local := ast.GetSourceFileOfNode(node).Locals[right.Text()]; local != nil && local.ValueDeclaration != nil {
		for _, decorator := range local.ValueDeclaration.ModifierNodes() {
			if !ast.IsDecorator(decorator) || !ast.IsCallExpression(decorator.Expression()) {
				continue
			}
			call := decorator.Expression()
			if !ast.IsIdentifier(call.Expression()) || len(call.Arguments()) == 0 || !ast.IsIdentifier(call.Arguments()[0]) {
				continue
			}
			name := call.Expression().Text()
			if (options.Extend.Decorator.Contains(name) || options.Extend.Decorator.IsZero() && strings.Contains("Extend", name)) && ast.IsIdentifier(root.Expression()) && call.Arguments()[0].Text() == root.Expression().Text() {
				symbol = local
			}
		}
		if ast.HasEtsStylesDecorator(local.ValueDeclaration, options) {
			symbol = local
		}
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if ast.IsStructDeclaration(parent) {
			if member := parent.Symbol().Members[right.Text()]; member != nil && member.ValueDeclaration != nil && ast.HasEtsStylesDecorator(member.ValueDeclaration, options) {
				symbol = member
			}
			break
		}
	}
	return symbol
}

func etsComponentRoot(root *ast.Node) *ast.Node {
	for ast.IsCallExpression(root) || ast.IsPropertyAccessExpression(root) {
		if ast.IsEtsComponentExpression(root) {
			return root
		}
		root = root.Expression()
	}
	return nil
}

func (c *Checker) isSystemEtsComponent(node *ast.Node) bool {
	if !ast.IsIdentifier(node) || !c.compilerOptions.Ets.Components.Contains(node.Text()) || c.compilerOptions.EtsLoaderPath == "" {
		return false
	}
	symbol := c.resolveSymbol(c.getSymbolAtLocation(node, false /*ignoreErrors*/))
	if symbol == nil || len(symbol.Declarations) != 1 {
		return false
	}
	declarationFile := ast.GetSourceFileOfNode(symbol.Declarations[0])
	if declarationFile == nil {
		return false
	}
	declarationsPath := tspath.GetNormalizedAbsolutePath("declarations", c.compilerOptions.EtsLoaderPath)
	return strings.HasPrefix(tspath.NormalizePath(declarationFile.FileName()), declarationsPath)
}

func (c *Checker) isInBuildOrPageTransitionContext(node *ast.Node) bool {
	if c.compilerOptions.Ets.Render.Method.IsZero() && c.compilerOptions.Ets.Render.Decorator.IsZero() {
		return false
	}
	for container := ast.GetContainingFunction(node); container != nil; container = ast.GetContainingFunction(container) {
		if ast.IsMethodDeclaration(container) {
			containingClass := ast.GetContainingClass(container)
			if ast.IsStructDeclaration(containingClass) && container.Name() != nil &&
				c.compilerOptions.Ets.Render.Method.Contains(container.Name().Text()) {
				return true
			}
		}
		if ast.IsMethodDeclaration(container) || ast.IsFunctionDeclaration(container) {
			for name := range c.compilerOptions.Ets.Render.Decorator.Values() {
				if ast.HasArkUIBareDecorator(container.Modifiers(), name) {
					return true
				}
			}
		}
	}
	return false
}
