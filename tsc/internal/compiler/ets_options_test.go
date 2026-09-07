package compiler_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

// OH parser.parseStructMembers and checker.resolveCallExpression,
// checkPropertyNotUsedBeforeDeclaration, checkGrammarDecorators and
// checkAllCodePathsInNonVoidFunctionReturnOrThrow own these contracts.
func TestEtsConfiguredSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, config, source string
		strict               bool
		codes                []int32
	}{
		{"ordinary struct call", `{}`, `struct S { value: number = 1; } S({value: 2});`, false, nil},
		{"ordinary class is not callable", `{}`, `class S {} S();`, false, []int32{2348}},
		{"explicit bag type", `{}`, `struct S { value: number = 1; } S({value: "bad"});`, false, []int32{2322}},
		{"initializer does not infer bag type", `{}`, `struct S { value = 1; } S({value: "allowed"});`, false, nil},
		{"configured base", `{"customComponent":"Base"}`, `declare class Base { height(n: number): this; } struct S {} S().height(1);`, false, nil},
		{"no invented common base", `{}`, `declare class CommonAttribute { height(n: number): this; } struct S {} S().height(1);`, false, []int32{2339}},
		{"explicit base wins", `{"customComponent":"MissingBase"}`, `class Base { height(n: number): this { return this; } } struct S extends Base {} S().height(1);`, false, nil},
		{"configured builder", `{"render":{"decorator":["Paint"]},"components":["widget"]}`, `declare const Paint: MethodDecorator; declare function widget(): void; @Paint function f() { widget() {} }`, false, nil},
		{"missing builder symbol", `{"render":{"decorator":["Paint"]}}`, `@Paint function f() {}`, false, []int32{2304}},
		{"SDK decorator signature checked", `{}`, `declare const Component: number; @Component struct S {}`, false, []int32{1238}},
		{"configured style return", `{"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `declare const Paint: MethodDecorator; declare const BaseInstance: {width(n: number): void}; @Paint function f() { .width(1) }`, false, nil},
		{"style argument checked", `{"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `declare const Paint: MethodDecorator; declare const BaseInstance: {width(n: number): void}; @Paint function f() { .width("bad") }`, false, []int32{2345}},
		{"style explicit return checked", `{"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `declare const Paint: MethodDecorator; @Paint function f(): number { return 1; }`, false, []int32{28003}},
		{"style undefined return checked", `{"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `declare const Paint: MethodDecorator; @Paint function f(): undefined {}`, false, []int32{28003}},
		{"property decoration is not strict initialization exemption", `{"propertyDecorators":[{"name":"Inject","needInitialization":false}]}`, `declare const Inject: PropertyDecorator; struct S { @Inject value: number; }`, true, []int32{2564}},
		{"configured before-use exemption", `{"propertyDecorators":[{"name":"Inject","needInitialization":false}]}`, `declare const Inject: PropertyDecorator; struct S { first = this.value; @Inject value: number = 1; }`, false, nil},
		{"configured before-use required", `{"propertyDecorators":[{"name":"Inject","needInitialization":true}]}`, `declare const Inject: PropertyDecorator; struct S { first = this.value; @Inject value: number = 1; }`, false, []int32{2729}},
		{"decorator call does not exempt initialization", `{"propertyDecorators":[{"name":"Inject","needInitialization":false}]}`, `declare function Inject(): PropertyDecorator; struct S { first = this.value; @Inject() value: number = 1; }`, false, []int32{2729}},
		{"dollar identifiers are not inferred aliases", `{}`, `struct S { message = "hi"; build() { $message; $$this.message; } }`, false, []int32{2304, 2304}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ets core.EtsOptions
			if err := json.Unmarshal([]byte(tc.config), &ets); err != nil {
				t.Fatal(err)
			}
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/input.ets": `declare class LocalStorage { private brand: never; } ` + tc.source}, true))
			options := &core.CompilerOptions{Ets: ets, Lib: []string{"lib.es2020.d.ts"}, NoEmit: core.TSTrue, ExperimentalDecorators: core.TSTrue, Strict: core.IfElse(tc.strict, core.TSTrue, core.TSFalse)}
			program := compiler.NewProgram(compiler.ProgramOptions{Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/input.ets"}, CompilerOptions: options}}, Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil)})
			if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
				t.Fatal(ds)
			}
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			var codes []int32
			for _, d := range ds {
				codes = append(codes, d.Code())
			}
			if !slices.Equal(codes, tc.codes) {
				t.Fatalf("expected %v; got %v: %v", tc.codes, codes, ds)
			}
		})
	}
}
