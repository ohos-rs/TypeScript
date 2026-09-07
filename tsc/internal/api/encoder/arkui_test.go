package encoder_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/api/encoder"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
)

func TestArkUIProtocolRoundTrip(t *testing.T) {
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/Page.ets", Path: "/Page.ets"}, `@Component struct Page { build() { Column() { Text("hello") }.width(100) } }`, core.ScriptKindETS)
	encoded, _, err := encoder.EncodeSourceFile(file)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := encoder.DecodeSourceFile(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ScriptKind != core.ScriptKindETS || !ast.IsStructDeclaration(decoded.Statements.Nodes[0]) {
		t.Fatal("ETS metadata lost in protocol")
	}
	p := printer.NewPrinter(printer.PrinterOptions{}, printer.PrintHandlers{}, nil)
	if p.EmitSourceFile(file) != p.EmitSourceFile(decoded) {
		t.Fatal("ArkUI body lost in protocol")
	}
	encoded[encoder.HeaderOffsetMetadata+3] = 8
	if _, err := encoder.DecodeSourceFile(encoded); err == nil {
		t.Fatal("accepted incompatible AST protocol")
	}
}
