package compiler_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

// OH parser.ts::parseClassElement injects readonly for Param without Once,
// and for Env/CustomEnv calls. The normal checker owns assignment diagnostics.
func TestArkUIStructReadonly(t *testing.T) {
	for _, tc := range []struct {
		decorators string
		readonly   bool
	}{
		{"@Param", true},
		{"@Param @Once", false},
		{"@Once @Param", false},
		{"@State", false},
		{"@Env('key')", true},
		{"@CustomEnv('key')", true},
		{"@Env('key') @Once", true},
		{"@Param readonly", true},
	} {
		t.Run(tc.decorators, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
				"/page.ets": `declare const Param: PropertyDecorator, Once: PropertyDecorator, State: PropertyDecorator; declare function Env(key: string): PropertyDecorator; declare function CustomEnv(key: string): PropertyDecorator; struct Page { ` + tc.decorators + ` value: number = 0; change() { this.value = 1; } build() {} }`,
			}, true))
			program := compiler.NewProgram(compiler.ProgramOptions{
				Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/page.ets"}, CompilerOptions: &core.CompilerOptions{NoEmit: core.TSTrue, ExperimentalDecorators: core.TSTrue}}},
				Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
			})
			if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
				t.Fatal(ds)
			}
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			if tc.readonly {
				if len(ds) != 1 || ds[0].Code() != 2540 {
					t.Fatalf("expected readonly assignment error, got %v", ds)
				}
			} else if len(ds) != 0 {
				t.Fatal(ds)
			}
		})
	}
}
