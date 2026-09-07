package parser_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
)

func TestArkUI(t *testing.T) {
	for _, source := range []string{
		`@Entry @Component struct Page {
		  @State message: string = "hello";
		  build() {
		    Column() {
		      Text(this.message).fontSize(20)
		      if (this.message) { Row() { Text("nested") } }
		      ForEach([1, 2], (n: number) => { Row() { Text(n.toString()) } })
		    }.width("100%")
		  }
		}`,
		`@Builder export function content() { Column() { Text("hello") } }
		 export default struct Page { build() { content() } }`,
		`export declare struct Widget { value: string; build(): void; }`,
		`@Styles function common() { .width(100).height(100) }
		 @Extend(Text) function emphasis() { .fontSize(24) }`,
		`const struct = 1; function f() { return struct; }
		 struct Page { @Builder part() { Column() {} } build() { this.part() } }`,
		`struct Page { build() { Repeat([1]).each((item) => { Row() { Text(item) } }) } }`,
		`struct Page { build() { Text($$this.message).stateStyles({ normal: { .fontSize(20).width(100) } }) } }`,
	} {
		t.Run(source, func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/page.ets", Path: "/page.ets"}, source, core.ScriptKindETS)
			if len(file.Diagnostics()) != 0 {
				t.Fatalf("parse diagnostics: %v", file.Diagnostics())
			}
			p := printer.NewPrinter(printer.PrinterOptions{NewLine: core.NewLineKindLF}, printer.PrintHandlers{}, nil)
			text := p.EmitSourceFile(file)
			reparsed := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/page.ets", Path: "/page.ets"}, text, core.ScriptKindETS)
			if len(reparsed.Diagnostics()) != 0 {
				t.Fatalf("round trip diagnostics: %v\n%s", reparsed.Diagnostics(), text)
			}
			counts := func(f *ast.SourceFile) [3]int {
				var result [3]int
				var visit ast.Visitor
				visit = func(n *ast.Node) bool {
					if ast.IsStructDeclaration(n) {
						result[0]++
					}
					if ast.IsEtsComponentExpression(n) {
						result[1]++
					}
					if ast.IsCallExpression(n) && n.AsCallExpression().EtsBody != nil {
						result[2]++
					}
					n.ForEachChild(visit)
					return false
				}
				visit(f.AsNode())
				return result
			}
			if counts(file) != counts(reparsed) {
				t.Fatalf("AST lost on round trip: %v -> %v\n%s", counts(file), counts(reparsed), text)
			}
			factory := &ast.NodeFactory{}
			clone := factory.DeepCloneReparse(file.AsNode()).AsSourceFile()
			if counts(file) != counts(clone) {
				t.Fatal("ArkUI nodes lost during cloning")
			}
		})
	}
}

func TestArkUIContextIsolation(t *testing.T) {
	for _, source := range []string{
		`function ordinary() { Column() {}.width(100) }`,
		`class Ordinary { build() { Column() {}.width(100) } }`,
		`struct Page { build() { ForEach(() => { Column() {}.width(100) }, item => {}) } }`,
		`struct Page { ordinary() { Column() {}.width(100) } }`,
		`struct Page { build() { Text("hello").onClick(() => { Column() {}.width(100) }) } }`,
		`struct Page { build() { function ordinary() { Column() {}.width(100) } } }`,
	} {
		file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/page.ets", Path: "/page.ets"}, source, core.ScriptKindETS)
		if len(file.Diagnostics()) == 0 {
			t.Errorf("ArkUI leaked into ordinary function: %s", source)
		}
	}
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/page.ts", Path: "/page.ts"}, `struct Page { build() { Column() {} } }`, core.ScriptKindTS)
	if len(file.Diagnostics()) == 0 {
		t.Error("TypeScript accepted struct syntax")
	}
}
