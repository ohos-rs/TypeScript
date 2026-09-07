package compiler_test

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

// OH checker.ts::getTypeAtFlowNode compares depth to ohApi.getMaxFlowDepth.
// The build host validates the range; the compiler consumes its numeric value.
func TestArkTSConfiguredFlowDepth(t *testing.T) {
	source := "export function f(x: string | number, condition: boolean) {\n" +
		strings.Repeat("if (condition) { x = 1; }\n", 2050) + "return x; }"
	for _, tc := range []struct {
		name  string
		limit float64
		fails bool
	}{
		{"default", 0, true},
		{"explicit default", 2000, true},
		{"increased", 65535, false},
		{"fractional not rounded", 2000.5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/flow.ets": source}, true))
			program := compiler.NewProgram(compiler.ProgramOptions{
				Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
					FileNames: []string{"/flow.ets"}, CompilerOptions: &core.CompilerOptions{NoEmit: core.TSTrue, MaxFlowDepth: tc.limit},
				}},
				Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
			})
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			if tc.fails {
				if len(ds) != 1 || ds[0].Code() != 2563 {
					t.Fatalf("expected flow depth error, got %v", ds)
				}
			} else if len(ds) != 0 {
				t.Fatal(ds)
			}
		})
	}
}
