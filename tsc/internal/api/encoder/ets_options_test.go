package encoder_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/api/encoder"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
)

func TestEtsOptionsProtocolIdentity(t *testing.T) {
	options := ast.SourceFileParseOptions{FileName: "/test.ets", Path: "/test.ets", Ets: etstest.Options(), EtsAnnotationsEnable: true}
	file := parser.ParseSourceFile(options, `@Builder function f() { Column() { Text("ok") } }`, core.ScriptKindETS)
	if len(file.Diagnostics()) != 0 {
		t.Fatal(file.Diagnostics())
	}
	data, _, err := encoder.EncodeSourceFile(file)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := encoder.DecodeSourceFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ParseOptions() != options {
		t.Fatal("ETS tables lost in AST protocol")
	}
	if !ast.IsEtsComponentExpression(decoded.Statements.Nodes[0].Body().Statements()[0].Expression()) {
		t.Fatal("component flag lost")
	}
}
