package parser_test

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
)

// OpenHarmony parser.ts::isSetLazy accepts exactly these import-clause shapes
// and stores the modifier independently from import type.
func TestOHImportLazySyntax(t *testing.T) {
	for _, source := range []string{
		`import lazy { value } from "mod";`,
		`import lazy value from "mod";`,
		`import lazy value, { other } from "mod";`,
	} {
		t.Run(source, func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/input.ets", Path: "/input.ets"}, source, core.ScriptKindETS)
			if len(file.Diagnostics()) != 0 {
				t.Fatalf("syntax diagnostics: %v", file.Diagnostics())
			}
			clause := file.Statements.Nodes[0].AsImportDeclaration().ImportClause.AsImportClause()
			if !clause.IsLazy || clause.PhaseModifier != ast.KindUnknown {
				t.Fatalf("import clause = %#v, want lazy non-type import", clause)
			}
			printed := printer.NewPrinter(printer.PrinterOptions{NewLine: core.NewLineKindLF}, printer.PrintHandlers{}, nil).EmitSourceFile(file)
			if !strings.Contains(printed, "import lazy ") {
				t.Fatalf("lazy modifier was not preserved: %s", printed)
			}
		})
	}
}

func TestOHImportLazyIsContextual(t *testing.T) {
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/input.ets", Path: "/input.ets"}, `import lazy from "mod";`, core.ScriptKindETS)
	if len(file.Diagnostics()) != 0 {
		t.Fatalf("syntax diagnostics: %v", file.Diagnostics())
	}
	clause := file.Statements.Nodes[0].AsImportDeclaration().ImportClause.AsImportClause()
	if clause.IsLazy || clause.Name() == nil || clause.Name().Text() != "lazy" {
		t.Fatalf("ordinary default binding was parsed as import lazy: %#v", clause)
	}
}
