package parser_test

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
)

func TestArkUIAnnotationSyntax(t *testing.T) {
	for _, tc := range []struct {
		source  string
		invalid bool
	}{
		{`@interface Empty {}`, false},
		{`export @interface Retention { policy: RetentionPolicy; }`, false},
		{`export declare @interface Values { count = 42; label: string = "arkts"; enabled: boolean = true }`, false},
		{`@interface await {} @interface C\u0032 {}`, false},
		{`@ interface Spaced {}`, true},
		{`@/*gap*/interface Spaced {}`, true},
		{`@interface A { ; }`, true},
		{`@interface A { private value: number }`, true},
		{`@interface A { get value(): number {} }`, true},
		{`@interface A { value?: number }`, true},
		{`@interface A { constructor() {} }`, true},
		{`@interface A { method(): void }`, true},
		{`@interface A { [key: string]: number }`, true},
		{`@interface A { 'value': number }`, true},
		{`class C { @interface A {} }`, true},
		{`declare let interface: PropertyDecorator; declare let other: PropertyDecorator; class Invalid { @interface @other value: number = 1 }`, true},
	} {
		t.Run(tc.source, func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/input.ets", Path: "/input.ets", EtsAnnotationsEnable: true}, tc.source, core.ScriptKindETS)
			if (len(file.Diagnostics()) != 0) != tc.invalid {
				t.Fatalf("syntax diagnostics: %v", file.Diagnostics())
			}
			if tc.invalid {
				return
			}
			if !ast.IsAnnotationDeclaration(file.Statements.Nodes[0]) {
				t.Fatal("annotation identity lost")
			}
			clone := (&ast.NodeFactory{}).DeepCloneReparse(file.AsNode()).AsSourceFile()
			if !ast.IsAnnotationDeclaration(clone.Statements.Nodes[0]) {
				t.Fatal("clone lost annotation flag")
			}
			p := printer.NewPrinter(printer.PrinterOptions{NewLine: core.NewLineKindLF}, printer.PrintHandlers{}, nil)
			text := p.EmitSourceFile(clone)
			if !strings.Contains(text, "@interface") {
				t.Fatalf("printed as an ordinary class: %s", text)
			}
			reparsed := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/input.d.ets", Path: "/input.d.ets", EtsAnnotationsEnable: true}, text, core.ScriptKindETS)
			if len(reparsed.Diagnostics()) != 0 || !ast.IsAnnotationDeclaration(reparsed.Statements.Nodes[0]) {
				t.Fatalf("round trip failed: %s", text)
			}
		})
	}
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/input.ts", Path: "/input.ts", EtsAnnotationsEnable: true}, `export @interface A {}`, core.ScriptKindTS)
	if len(file.Diagnostics()) == 0 {
		t.Fatal("annotation grammar leaked into TypeScript")
	}
}
