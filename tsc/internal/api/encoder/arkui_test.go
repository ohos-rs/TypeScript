package encoder_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/api/encoder"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/printer"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
)

func TestArkUIProtocolRoundTrip(t *testing.T) {
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{Ets: etstest.Options(), FileName: "/Page.ets", Path: "/Page.ets"}, `@Component struct Page { build() { Column() { Text("hello") }.width(100) } }`, core.ScriptKindETS)
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
	structNode := decoded.Statements.Nodes[0]
	constructor := structNode.Members()[0]
	if !ast.IsConstructorDeclaration(constructor) || !constructor.Virtual || !constructor.Parameters()[0].Name().Virtual || constructor.Parameters()[0].Name().Text() != "##storage" {
		t.Fatal("virtual constructor metadata lost in protocol")
	}
	body := structNode.Members()[1].Body().Statements()[0].Expression().Expression().Expression().AsCallExpression().EtsBody
	if body == nil || len(body.Statements()) != 1 || !ast.IsEtsComponentExpression(body.Statements()[0].Expression()) {
		t.Fatal("ArkUI body lost in protocol")
	}
	encoded[encoder.HeaderOffsetMetadata+3] = 8
	if _, err := encoder.DecodeSourceFile(encoded); err == nil {
		t.Fatal("accepted incompatible AST protocol")
	}
}

func TestArkUIAnnotationProtocolRoundTrip(t *testing.T) {
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{Ets: etstest.Options(), FileName: "/annotation.d.ets", Path: "/annotation.d.ets", EtsAnnotationsEnable: true}, `export declare @interface A { value: number = 1; }`, core.ScriptKindETS)
	encoded, _, err := encoder.EncodeSourceFile(file)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := encoder.DecodeSourceFile(encoded)
	if err != nil {
		t.Fatal(err)
	}
	declaration := decoded.Statements.Nodes[0]
	if decoded.ParseOptions() != file.ParseOptions() {
		t.Fatal("annotation parse identity lost in API encoding")
	}
	if !ast.IsAnnotationDeclaration(declaration) || !ast.IsAnnotationPropertyDeclaration(declaration.Members()[0]) {
		t.Fatal("annotation identity lost in API encoding")
	}
	p := printer.NewPrinter(printer.PrinterOptions{}, printer.PrintHandlers{}, nil)
	if p.EmitSourceFile(file) != p.EmitSourceFile(decoded) {
		t.Fatal("annotation data lost in API encoding")
	}
}
