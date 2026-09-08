package compiler_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

func TestArkUIAnnotationDeclarationEmit(t *testing.T) {
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/annotation.ets": `export @interface Values { count = 42; label: string = "arkts"; }`,
	}, true))
	options := &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, Declaration: core.TSTrue, EmitDeclarationOnly: core.TSTrue, Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/annotation.ets"}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	written := make(map[string]string)
	result := program.Emit(t.Context(), compiler.EmitOptions{WriteFile: func(name, text string, _ *compiler.WriteFileData) error { written[name] = text; return nil }})
	if result.EmitSkipped || len(result.Diagnostics) != 0 {
		t.Fatal(result.Diagnostics)
	}
	text := written["/annotation.d.ets"]
	for _, expected := range []string{"@interface Values", "count: number = 42", `label: string = "arkts"`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("lost annotation declaration contract %q: %s", expected, text)
		}
	}
}

func TestArkUIAnnotationDeclarationRetainsPolicyAndImports(t *testing.T) {
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/@arkts.lang.d.ets": `export declare const enum RetentionPolicy { SOURCE = "source" } export declare @interface Retention { policy: RetentionPolicy; }`,
		"/annotation.ets": `import { Retention, RetentionPolicy } from './@arkts.lang';
@Retention({policy: RetentionPolicy.SOURCE}) export @interface Source { value = 42; }
@Source export interface I { @Source value: number; }
@Source export function f(): number { return 1; }
@Source export class C { @Source method() {} }`,
	}, true))
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/annotation.ets"}, CompilerOptions: &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, Declaration: core.TSTrue, EmitDeclarationOnly: core.TSTrue, Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext, ModuleResolution: core.ModuleResolutionKindBundler}}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if ds := program.GetSemanticDiagnostics(t.Context(), nil); len(ds) != 0 {
		t.Fatal(ds)
	}
	written := make(map[string]string)
	result := program.Emit(t.Context(), compiler.EmitOptions{WriteFile: func(name, text string, _ *compiler.WriteFileData) error { written[name] = text; return nil }})
	if result.EmitSkipped || len(result.Diagnostics) != 0 {
		t.Fatal(result.Diagnostics)
	}
	text := written["/annotation.d.ets"]
	for _, expected := range []string{"import { Retention, RetentionPolicy }", "@Retention({ policy: RetentionPolicy.SOURCE })", "@interface Source", "value: number = 42", "@Source"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in declaration:\n%s", expected, text)
		}
	}
	if strings.Count(text, "@Source") != 5 {
		t.Fatalf("lost declaration annotation targets:\n%s", text)
	}
}

func TestArkUIImportedAnnotationDoesNotEmitJSDecorator(t *testing.T) {
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/types.d.ets": `export declare @interface A { value: number = 1; }`,
		"/entry.ets":   `import { A } from "./types"; @A export class C {}`,
	}, true))
	options := &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext, ModuleResolution: core.ModuleResolutionKindBundler}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/entry.ets"}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if ds := program.GetSemanticDiagnostics(t.Context(), nil); len(ds) != 0 {
		t.Fatal(ds)
	}
	written := false
	result := program.Emit(t.Context(), compiler.EmitOptions{WriteFile: func(string, string, *compiler.WriteFileData) error { written = true; return nil }})
	if written || !result.EmitSkipped || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code() != 100069 {
		t.Fatalf("invalid JS annotation emission: %+v", result)
	}
}

// OH hasSourceRetentionPolicy/checkSourceRetentionAnnotation distinguish SDK
// Retention by declaration identity; a user-defined homonym has no effect.
func TestArkUIAnnotationSourceRetention(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		code         int32
	}{
		{"function", `@Source function f() {}`, 0},
		{"variable", `@Source const x = 1;`, 0},
		{"interface", `@Source interface I { @Source value: number; @Source method(): void; }`, 0},
		{"type", `@Source type T = number;`, 0},
		{"enum", `@Source enum E { A }`, 0},
		{"namespace", `@Source namespace N { export const value = 1; }`, 0},
		{"abstract", `@Source abstract class C { @Source abstract method(): void; @Source value: number = 0; }`, 0},
		{"accessors", `class C { @Source get value() { return 1; } @Source set value(n: number) {} }`, 0},
		{"annotation", `@Source @interface Another {}`, 0},
		{"constructor", `class C { @Source constructor() {} }`, 28043},
		// OH parseTypeMember consumes but drops decorators on index signatures.
		{"index signature", `interface I { @Source readonly [key: string]: number; }`, 0},
		{"parameter", `function f(@Source value: number) {}`, 28043},
		{"duplicate", `@Source @Source function f() {}`, 28023},
		{"runtime", `@Retention({policy: RetentionPolicy.RUNTIME}) @interface Runtime {} @Runtime function f() {}`, 28022},
		{"quoted-policy", `@Retention({'policy': RetentionPolicy.SOURCE}) @interface Quoted {} @Quoted function f() {}`, 28022},
		{"enum-identity", `const enum Custom { SOURCE = 'source' } @Retention({policy: Custom.SOURCE}) @interface CustomPolicy {} @CustomPolicy function f() {}`, 0},
		{"sdk-available", `import { Available } from './@ohos.annotation'; @Available({minApiVersion: '26.0.0'}) function f() {}`, 0},
		{"sdk-suppress", `import { SuppressWarnings, Rules } from './@ohos.annotation'; @SuppressWarnings({rules: [Rules.API]}) interface I {}`, 0},
		{"available-homonym", `@interface Available {minApiVersion: string;} @Available({minApiVersion: '26.0.0'}) function f() {}`, 28022},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
				"/@arkts.lang.d.ets":      `export declare const enum RetentionPolicy { SOURCE = "source", RUNTIME = "runtime" } export declare @interface Retention { policy: string; }`,
				"/@ohos.annotation.d.ets": `export declare @interface Available { minApiVersion: string; } export declare const enum Rules { API = 'api' } export declare @interface SuppressWarnings { rules: Rules[]; }`,
				"/entry.ets": `import { Retention, RetentionPolicy } from './@arkts.lang';
@Retention({policy: RetentionPolicy.SOURCE}) @interface Source {}
` + tc.source,
			}, true))
			program := compiler.NewProgram(compiler.ProgramOptions{
				Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/entry.ets"}, CompilerOptions: &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, NoEmit: core.TSTrue, Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext, ModuleResolution: core.ModuleResolutionKindBundler}}},
				Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
			})
			if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
				t.Fatalf("syntax: %v", ds)
			}
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			codes := make([]int32, 0, len(ds))
			for _, d := range ds {
				codes = append(codes, d.Code())
			}
			if tc.code == 0 && len(ds) != 0 || tc.code != 0 && !slices.Contains(codes, tc.code) {
				t.Fatalf("expected %d, got %v (%v)", tc.code, codes, ds)
			}
		})
	}
}

func TestArkUIAnnotationJsHarScope(t *testing.T) {
	for _, tc := range []struct {
		name, root, entry string
		enabled           core.Tristate
		want              int
	}{
		{"declaration-in-module", "/module", "/module/entry.ets", core.TSTrue, 1},
		{"declaration-outside-module", "/other", "/module/entry.ets", core.TSTrue, 0},
		{"empty-root", "", "/module/entry.ets", core.TSTrue, 0},
		{"normalized-root", "/module/child/..", "/module/entry.ets", core.TSTrue, 1},
		{"source-prefix-contract", "/module", "/module-extra/entry.ets", core.TSTrue, 1},
		{"not-js-har", "/module", "/module/entry.ets", core.TSFalse, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{tc.entry: `export @interface A {}`}, true))
			program := compiler.NewProgram(compiler.ProgramOptions{
				Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{tc.entry}, CompilerOptions: &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, NoEmit: core.TSTrue, IsCompileJsHar: tc.enabled, ModuleRootPath: tc.root}}},
				Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
			})
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			if len(ds) != tc.want || len(ds) > 0 && ds[0].Code() != 28041 {
				t.Fatalf("want %d HAR diagnostics, got %v", tc.want, ds)
			}
		})
	}
}

// OH checker.ts annotation declaration/property rules, with SDK-style enum
// declarations. Positive tests check semantics too (skipLibCheck is disabled).
func TestArkUIAnnotationChecking(t *testing.T) {
	for _, tc := range []struct {
		source string
		code   int32
	}{
		{`export @interface Retention { policy: RetentionPolicy; }`, 0},
		{`@interface A { value: number = 1 } @A class C { @A() method() {} }`, 0},
		{`@interface A { value: number; other: string = "ok" } @A({ value: 2 }) class C {}`, 0},
		{`@interface A { value: number } @A class C {}`, 28019},
		{`@interface A { value: number } @A(1) class C {}`, 28020},
		{`@interface A { value: number } @A({ value:"bad" }) class C {}`, 2322},
		{`@interface A {} @A @A class C {}`, 28023},
		{`@interface A {} @A abstract class C {}`, 28021},
		{`@interface A {} class C { @A value: number }`, 28022},
		{`@interface A { count = 42; text: string = "ok"; flag: boolean = !false; values: number[][] = [[1 + 2]] }`, 0},
		{`const N = 2; @interface A { value: number = N * 4; policy: RetentionPolicy = RetentionPolicy.RUNTIME }`, 0},
		{`@interface A { value: string = "a" + "b"; flag: boolean = (1 < 2) && true }`, 0},
		{`@interface A { value: object }`, 28033},
		{`@interface A { value: number | string }`, 28033},
		{`@interface A { value: [number] }`, 28033},
		{`@interface A { value }`, 28032},
		{`function f(): number { return 1 } @interface A { value: number = f() }`, 28034},
		{`let x = 1; @interface A { value: number = x }`, 28034},
		{`@interface A { value: string = "a" + 1 }`, 28034},
		{`function f() { @interface A {} }`, 28024},
		{`@interface A {} let value: A;`, 28028},
		{`@interface A {} const value = A;`, 28027},
		{`@interface A {} const value = { annotation: A };`, 28027},
		{`import { Imported } from './imported'; const value = { annotation: Imported };`, 28029},
		{`import { Imported } from './imported'; const value = { Imported };`, 28029},
		{`@A class C {} @interface A {}`, 28036},
		{`@A() class C {} @interface A {}`, 28036},
		{`@A({ value: 1 }) class C {} @interface A { value: number }`, 28036},
		{`@A class C {} declare @interface A {}`, 0},
		{`function f() { @A class C {} } @interface A {}`, 0},
		{`@interface A {} const value = new A();`, 28027},
		{`export default @interface A {}`, 28031},
	} {
		t.Run(tc.source, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
				"/tsconfig.json":  `{"compilerOptions":{"etsAnnotationsEnable":true,"noEmit":true,"lib":["es2020"],"strict":false,"module":"esnext","moduleResolution":"bundler"},"include":["**/*"]}`,
				"/sdk.d.ets":      `declare const enum RetentionPolicy { SOURCE, RUNTIME }`,
				"/imported.d.ets": `export declare @interface Imported {}`,
				"/input.ets":      tc.source,
			}, true))
			host := compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil)
			config, errors := tsoptions.GetParsedCommandLineOfConfigFile("/tsconfig.json", &core.CompilerOptions{}, nil, host, nil)
			if len(errors) != 0 {
				t.Fatal(errors)
			}
			program := compiler.NewProgram(compiler.ProgramOptions{Config: config, Host: host})
			if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
				t.Fatalf("syntax: %v", ds)
			}
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			codes := make([]int32, 0, len(ds))
			for _, d := range ds {
				codes = append(codes, d.Code())
			}
			if tc.code == 0 && len(ds) != 0 || tc.code != 0 && !slices.Contains(codes, tc.code) {
				t.Fatalf("expected code %d, got %v (%v)", tc.code, codes, ds)
			}
		})
	}
}

// OH annotationApplicationError13, annotationDeclarationError2, and
// AnnotationDeclarationNegative19 have one placement diagnostic and no
// temporal-dead-zone cascade for an annotation declaration annotating itself.
func TestArkUIAnnotationSelfApplicationDoesNotCascade(t *testing.T) {
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/input.ets": "@A\n@interface A {}",
	}, true))
	options := &core.CompilerOptions{EtsAnnotationsEnable: core.TSTrue, NoEmit: core.TSTrue}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/input.ets"}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	diagnostics := program.GetSemanticDiagnostics(t.Context(), nil)
	if len(diagnostics) != 1 || diagnostics[0].Code() != 28026 {
		t.Fatalf("diagnostics = %v, want only TS28026", diagnostics)
	}
}

// OH retentionError1 covers the complete SDK Retention contract in one
// multi-file program, including target validation and BYTECODE policy.
func TestArkUIRetentionCorpusContract(t *testing.T) {
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/@arkts.lang.d.ets": `export const enum RetentionPolicy { SOURCE='source', BYTECODE='bytecode' }
export @interface Retention { policy: RetentionPolicy }`,
		"/A.ets": `import { RetentionPolicy, Retention } from "./@arkts.lang";
@Retention({policy: RetentionPolicy.SOURCE}) class A {}
@Retention @interface Anno1 {}
@Retention({policy: RetentionPolicy.SOURCE}) @interface Anno2 { value: number }
@Retention({policy: RetentionPolicy.BYTECODE}) @interface Anno3 { value: number }
@Anno3({value: 2}) let b = 2;`,
	}, true))
	options := &core.CompilerOptions{
		EtsAnnotationsEnable: core.TSTrue, NoEmit: core.TSTrue,
		Strict: core.TSFalse,
		Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext, ModuleResolution: core.ModuleResolutionKindBundler,
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/A.ets"}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	diagnostics := program.GetSemanticDiagnostics(t.Context(), nil)
	codes := make([]int32, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code())
	}
	if !slices.Equal(codes, []int32{28046, 28019, 28022}) {
		t.Fatalf("diagnostics = %v (%v)", codes, diagnostics)
	}
}
