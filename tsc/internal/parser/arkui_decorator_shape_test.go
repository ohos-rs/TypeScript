package parser_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
)

// OH isTokenInsideBuilder/hasEtsStylesDecoratorNames and
// getEtsExtendDecoratorsComponentNames distinguish decorator AST shapes.
func TestArkUIDecoratorShapesDoNotEnableUnrelatedContexts(t *testing.T) {
	for _, decorator := range []string{"@Builder()", "@Namespace.Builder", "@Extend"} {
		t.Run(decorator, func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{Ets: etstest.Options(), FileName: "/input.ets", Path: "/input.ets"}, decorator+"\nfunction f() {\nColumn()\n{}\n}", core.ScriptKindETS)
			if len(file.Diagnostics()) != 0 {
				t.Fatal(file.Diagnostics())
			}
			statements := file.Statements.Nodes[0].Body().Statements()
			if len(statements) != 2 || !ast.IsCallExpression(statements[0].Expression()) || ast.IsEtsComponentExpression(statements[0].Expression()) || !ast.IsBlock(statements[1]) {
				t.Fatal("ordinary call and block were reinterpreted as UI DSL")
			}
		})
	}
	for _, tc := range []struct {
		decorator string
		valid     bool
	}{
		{"@Styles", true}, {"@Styles()", false}, {"@Namespace.Styles", false},
		{"@Extend(Text)", true}, {"@AnimatableExtend(Text)", true},
		{"@Extend", false}, {"@Extend()", false}, {"@Extend('Text')", false},
	} {
		t.Run(tc.decorator+" leading dot", func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{Ets: etstest.Options(), FileName: "/input.ets", Path: "/input.ets"}, tc.decorator+"\nfunction f() {\n.fontSize(16)\n}", core.ScriptKindETS)
			if (len(file.Diagnostics()) == 0) != tc.valid {
				t.Fatalf("unexpected DSL activation: %v", file.Diagnostics())
			}
		})
	}
}
