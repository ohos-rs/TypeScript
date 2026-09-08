package compiler_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

func TestArkUIProgram(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		wantErrors       bool
	}{
		{"valid", `Card({ title: "hello" })`, false},
		{"wrong property type", `Card({ title: 42 })`, true},
		{"unknown property", `Card({ missing: true })`, true},
		{"nested error", `Column() { Text(42) }`, true},
		{"unknown member", `Text(this.missing)`, true},
		{"styles", `Text("hello").common().emphasis(20)`, false},
		{"styles argument", `Text("hello").emphasis("wrong")`, true},
		{"wrong styles target", `Column().emphasis(20)`, true},
		{"common attributes", `Card().width(100)`, false},
		{"state styles", `Text("hi").stateStyles({ normal: { .fontSize(20) } })`, false},
		{"state styles type error", `Text("hi").stateStyles({ normal: { .fontSize("bad") } })`, true},
		// ets2bundle collects props and filters name diagnostics in its build
		// host; OH TypeScript does not invent dollar-prefixed member aliases.
		{"dollar identifiers need declarations", `Card({ title: $message }); Text($$this.message)`, true},
		{"declared dollar identifier", `Text($r("app.string.title"))`, false},
		{"unknown binding", `Text($missing)`, true},
		{"generic component", `Box<string>({ value: "hello" })`, false},
		{"generic component error", `Box<string>({ value: 42 })`, true},
		// OH parseStructMembers makes all properties optional; the build
		// transform, not the TypeScript signature, enforces @Require.
		{"require is checked by build transform", `Box<string>()`, false},
		{"local storage parameter", `Card({ title: "hello" }, new LocalStorage())`, false},
		{"local storage type", `Card({}, 1)`, true},
		{"empty struct storage", `Empty(new LocalStorage())`, false},
		{"empty struct has no property bag", `Empty({}, new LocalStorage())`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
				"/tsconfig.json": `{"compilerOptions":{"noEmit":true,"lib":["es2020"],"strict":true,"module":"esnext","moduleResolution":"bundler","allowImportingTsExtensions":true},"include":["**/*"]}`,
				// Narrow declarations matching SDK common.d.ts/text.d.ts identities;
				// virtual receivers and CustomComponent are real SDK declarations.
				"/sdk.d.ets": `declare class LocalStorage { private storageBrand: never; } declare class CommonAttribute { width(value: number): this; } declare class CustomComponent extends CommonAttribute {} declare class TextAttribute extends CommonAttribute { fontSize(value: number): this; stateStyles(styles: { normal?: object }): this; } declare const CommonInstance: CommonAttribute; declare const TextInstance: TextAttribute; declare function Text(value: string): TextAttribute; declare function Column(): CommonAttribute; declare function $r(name: string): string; declare const Component: ClassDecorator, Entry: ClassDecorator; declare const Require: PropertyDecorator, Prop: PropertyDecorator, State: PropertyDecorator; declare const Styles: MethodDecorator;`,
				"/Card.ets":  `@Component export struct Box<T> { @Require @Prop value!: T; build() {} } @Component export struct Card { @Prop title: string = ""; build() { Text(this.title) } }`,
				"/Page.ets":  `import { Card, Box } from "./Card"; struct Empty { build() {} } @Styles function common() { .width(100) } @Extend(Text) function emphasis(size: number) { .fontSize(size) } @Entry @Component struct Page { @State message: string = "hello"; build() { ` + tc.expression + ` } }`,
			}, true))
			host := compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil)
			config, errors := tsoptions.GetParsedCommandLineOfConfigFile("/tsconfig.json", &core.CompilerOptions{Ets: etstest.Options(), ExperimentalDecorators: core.TSTrue}, nil, host, nil)
			if len(errors) != 0 {
				t.Fatalf("config errors: %v", errors)
			}
			program := compiler.NewProgram(compiler.ProgramOptions{Config: config, Host: host})
			for _, name := range []string{"/Card.ets", "/Page.ets", "/sdk.d.ets"} {
				file := program.GetSourceFile(name)
				if file == nil || file.ScriptKind != core.ScriptKindETS {
					t.Fatalf("ETS file not loaded: %s", name)
				}
			}
			if !program.GetSourceFile("/sdk.d.ets").IsDeclarationFile {
				t.Fatal(".d.ets is not a declaration file")
			}
			if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
				t.Fatalf("syntax errors: %v", ds)
			}
			ds := program.GetSemanticDiagnostics(t.Context(), nil)
			if (len(ds) > 0) != tc.wantErrors {
				t.Fatalf("semantic diagnostics (want errors %v): %v", tc.wantErrors, ds)
			}
			program.BindSourceFiles()
			c, done := program.GetTypeChecker(t.Context())
			defer done()
			card := program.GetSourceFile("/Card.ets").Statements.Nodes[0]
			if !ast.IsStructDeclaration(card) || c.GetSymbolAtLocation(card.Name()) == nil {
				t.Fatal("struct symbol missing")
			}
		})
	}
}

func TestArkUIStructExplicitConstructorUsesVirtualSignature(t *testing.T) {
	t.Parallel()

	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk.d.ets": `declare class LocalStorage {} declare class CustomComponent {}`,
		"/input.ets": `struct Page { value: string = ""; constructor() { super(); this.value = "ready"; } build() {} }`,
	}, true))
	options := &core.CompilerOptions{
		NoEmit:                 core.TSTrue,
		Module:                 core.ModuleKindESNext,
		ModuleResolution:       core.ModuleResolutionKindBundler,
		ExperimentalDecorators: core.TSTrue,
		Ets:                    etstest.Options(),
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       []string{"/sdk.d.ets", "/input.ets"},
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	for _, diagnostic := range program.GetSemanticDiagnostics(t.Context(), nil) {
		if diagnostic.Code() == 2392 {
			t.Fatalf("OH virtual constructor must not duplicate the explicit constructor: %v", diagnostic)
		}
	}
}

func TestArkUIUsesOpenHarmony49ExpressionDiagnostics(t *testing.T) {
	t.Parallel()

	const source = `
		declare class Box<T> { value: T; }
		declare const maybeBox: unknown;
		declare const arkLength: string | number;
		if ({}) {}
		if (null) {}
		if (arkLength > 0) {}
		maybeBox instanceof Box<number>;
	`
	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/input.ts": source}, true))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/input.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:           core.TSTrue,
					Target:           core.ScriptTargetESNext,
					Module:           core.ModuleKindESNext,
					ModuleResolution: core.ModuleResolutionKindBundler,
					EtsLoaderPath:    loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		var codes []int32
		for _, diagnostic := range program.GetSemanticDiagnostics(t.Context(), nil) {
			codes = append(codes, diagnostic.Code())
		}
		return codes
	}

	ordinary := check("")
	for _, code := range []int32{2365, 2848, 2872, 2873} {
		if !slices.Contains(ordinary, code) {
			t.Fatalf("ordinary TypeScript lost TS%d: %v", code, ordinary)
		}
	}
	openHarmony := check("/loader")
	for _, code := range []int32{2365, 2848, 2872, 2873} {
		if slices.Contains(openHarmony, code) {
			t.Fatalf("OpenHarmony 4.9 mode must not report TS%d: %v", code, openHarmony)
		}
	}
}

func TestArkUIUsesOpenHarmony49LegacyEscapeScanning(t *testing.T) {
	t.Parallel()

	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
			"/input.ts": `export const escape = "\033\8";`,
		}, true))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/input.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:        core.TSTrue,
					Target:        core.ScriptTargetESNext,
					EtsLoaderPath: loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		var codes []int32
		for _, diagnostic := range program.GetSyntacticDiagnostics(t.Context(), nil) {
			codes = append(codes, diagnostic.Code())
		}
		return codes
	}

	ordinary := check("")
	for _, code := range []int32{1487, 1488} {
		if !slices.Contains(ordinary, code) {
			t.Fatalf("ordinary TypeScript lost TS%d: %v", code, ordinary)
		}
	}
	if openHarmony := check("/loader"); len(openHarmony) != 0 {
		t.Fatalf("OpenHarmony 4.9 scanner reported newer escape diagnostics: %v", openHarmony)
	}
}

func TestArkUIUsesOpenHarmony49ComputedNumericEnumFlow(t *testing.T) {
	t.Parallel()

	const source = `
		enum ShareChannel {
			NONE = 0,
			WEIXIN = 1 << 0,
			POSTER = 1 << 13,
		}
		function select(channel: string): void {
			let shareType = ShareChannel.NONE;
			switch (channel) {
				case "weixin":
					shareType = ShareChannel.WEIXIN;
					break;
				default:
					return;
			}
			if (shareType === ShareChannel.POSTER) {}
		}
	`
	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/input.ts": source}, true))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/input.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:        core.TSTrue,
					Target:        core.ScriptTargetESNext,
					EtsLoaderPath: loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		var codes []int32
		for _, diagnostic := range program.GetSemanticDiagnostics(t.Context(), nil) {
			codes = append(codes, diagnostic.Code())
		}
		return codes
	}

	if ordinary := check(""); !slices.Contains(ordinary, 2367) {
		t.Fatalf("ordinary TypeScript lost the computed-enum narrowing diagnostic: %v", ordinary)
	}
	if openHarmony := check("/loader"); slices.Contains(openHarmony, 2367) {
		t.Fatalf("OpenHarmony 4.9 numeric enum must not narrow to member literals: %v", openHarmony)
	}
}

func TestArkUIUsesOpenHarmony49FileCasingDefault(t *testing.T) {
	t.Parallel()

	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
			"/entry.ts": `import "./Index"; import "./index";`,
			"/index.ts": `export const value = 1;`,
		}, false))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/entry.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:           core.TSTrue,
					Module:           core.ModuleKindESNext,
					ModuleResolution: core.ModuleResolutionKindBundler,
					EtsLoaderPath:    loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		var codes []int32
		for _, diagnostic := range program.GetSemanticDiagnostics(t.Context(), nil) {
			codes = append(codes, diagnostic.Code())
		}
		return codes
	}

	ordinary := check("")
	if !slices.Contains(ordinary, int32(1149)) && !slices.Contains(ordinary, int32(1261)) {
		t.Fatalf("ordinary TypeScript lost its default casing diagnostic: %v", ordinary)
	}
	openHarmony := check("/loader")
	if slices.Contains(openHarmony, int32(1149)) || slices.Contains(openHarmony, int32(1261)) {
		t.Fatalf("OpenHarmony 4.9 mode enabled casing checks by default: %v", openHarmony)
	}
}

func TestArkUIUsesOpenHarmony49IterableDeclarations(t *testing.T) {
	t.Parallel()

	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/lib.d.ts": `
			interface SymbolConstructor { readonly iterator: unique symbol; }
			declare var Symbol: SymbolConstructor;
			interface IteratorYieldResult<TYield> { done?: false; value: TYield; }
			interface IteratorReturnResult<TReturn> { done: true; value: TReturn; }
			type IteratorResult<T, TReturn = any> = IteratorYieldResult<T> | IteratorReturnResult<TReturn>;
			interface Iterator<T, TReturn = any, TNext = undefined> { next(...args: [] | [TNext]): IteratorResult<T, TReturn>; }
			interface Iterable<T> { [Symbol.iterator](): Iterator<T>; }
			interface IterableIterator<T> extends Iterator<T> { [Symbol.iterator](): IterableIterator<T>; }
			interface Set<T> extends Iterable<T> {}
			declare const values: Set<number>;
		`,
		"/input.ts": `for (const value of values) { const checked: number = value; }`,
	}, true))
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames: []string{"/lib.d.ts", "/input.ts"},
			CompilerOptions: &core.CompilerOptions{
				NoEmit:        core.TSTrue,
				NoLib:         core.TSTrue,
				Target:        core.ScriptTargetES2021,
				EtsLoaderPath: "/loader",
			},
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if diagnostics := program.GetSemanticDiagnostics(t.Context(), nil); len(diagnostics) != 0 {
		t.Fatalf("OH 4.9 Iterable<T> declarations must remain iterable: %v", diagnostics)
	}
}

func TestArkUIUsesOpenHarmony49NumericEnumAssignability(t *testing.T) {
	t.Parallel()

	const source = `
		enum CommunicationType { Request = 1, Response = 2 }
		declare function send(value: CommunicationType): void;
		send(-1);
		const value: CommunicationType = -1;
	`
	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/input.ts": source}, true))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/input.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:        core.TSTrue,
					Target:        core.ScriptTargetES2021,
					EtsLoaderPath: loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		return core.Map(program.GetSemanticDiagnostics(t.Context(), nil), func(diagnostic *ast.Diagnostic) int32 {
			return diagnostic.Code()
		})
	}

	ordinary := check("")
	for _, code := range []int32{2322, 2345} {
		if !slices.Contains(ordinary, code) {
			t.Fatalf("ordinary TypeScript lost TS%d: %v", code, ordinary)
		}
	}
	if openHarmony := check("/loader"); len(openHarmony) != 0 {
		t.Fatalf("OH 4.9 numeric enum assignments must remain compatible: %v", openHarmony)
	}
}

func TestArkUIUsesOpenHarmony49CommonSupertypeInference(t *testing.T) {
	t.Parallel()

	const source = `
		interface ContextA { a: number; }
		interface ContextB { b: number; }
		interface Extra { enabled: boolean; }
		abstract class ICanvasDraw<T extends object = object> {
			abstract methodName: string;
			abstract onDraw(ctx: ContextA | ContextB, data: T, extra: Extra): Promise<boolean>;
		}
		type ArcParams = [number, number, number];
		type DrawImageParams = [string, number, number] | [string, number, number, number, number];
		type ClipParams = number[];
		class Arc extends ICanvasDraw<ArcParams> {
			methodName = "arc";
			async onDraw(ctx: ContextA, data: ArcParams, extra: Extra): Promise<boolean> { return true; }
		}
		class DrawImage extends ICanvasDraw<DrawImageParams> {
			methodName = "drawImage";
			async onDraw(ctx: ContextA, data: DrawImageParams, extra: Extra): Promise<boolean> { return true; }
		}
		class Clip extends ICanvasDraw<ClipParams> {
			methodName = "clip";
			async onDraw(ctx: ContextB, data: ClipParams, extra: Extra): Promise<boolean> { return true; }
		}
		const methods: Map<string, ICanvasDraw<object>> = new Map([
			["arc", new Arc() as ICanvasDraw<object>],
			["drawImage", new DrawImage()],
			["clip", new Clip()],
		]);
	`
	check := func(loaderPath string) []int32 {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{"/input.ts": source}, true))
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
				FileNames: []string{"/input.ts"},
				CompilerOptions: &core.CompilerOptions{
					NoEmit:        core.TSTrue,
					Target:        core.ScriptTargetES2021,
					EtsLoaderPath: loaderPath,
				},
			}},
			Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		return core.Map(program.GetSemanticDiagnostics(t.Context(), nil), func(diagnostic *ast.Diagnostic) int32 {
			return diagnostic.Code()
		})
	}

	if ordinary := check(""); !slices.Contains(ordinary, int32(2769)) {
		t.Fatalf("ordinary TypeScript lost its newer common-supertype result: %v", ordinary)
	}
	if openHarmony := check("/loader"); len(openHarmony) != 0 {
		t.Fatalf("OH 4.9 common-supertype inference must accept the heterogeneous Map: %v", openHarmony)
	}
}

func TestArkUISourceOwnedDiagnostics(t *testing.T) {
	t.Parallel()

	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/loader/declarations/badge.d.ets": `declare class LocalStorage {} declare class CustomComponent {} declare function Badge(value: string): void;`,
		"/input.ets": `
			struct Column { build() {} }
			struct { build() {} }
			function invalidPlace() { Badge("outside") }
			struct Page { build() { Badge("inside") } }
		`,
	}, true))
	options := &core.CompilerOptions{
		NoEmit:                 core.TSTrue,
		Module:                 core.ModuleKindESNext,
		ModuleResolution:       core.ModuleResolutionKindBundler,
		ExperimentalDecorators: core.TSTrue,
		EtsAnnotationsEnable:   core.TSTrue,
		EtsLoaderPath:          "/loader",
		Ets:                    etstest.Options(),
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       []string{"/loader/declarations/badge.d.ets", "/input.ets"},
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	diagnostics := program.GetSemanticDiagnostics(t.Context(), nil)
	codes := make([]int32, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		codes = append(codes, diagnostic.Code())
	}
	for _, code := range []int32{28002, 28006, 28015} {
		if !slices.Contains(codes, code) {
			t.Errorf("missing TS%d in diagnostics %v", code, diagnostics)
		}
	}
}

func TestArkUIOhExportsDiagnostic(t *testing.T) {
	t.Parallel()

	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/repo/entry.ets":                       `import { hidden } from "pkg"; hidden();`,
		"/repo/oh_modules/pkg/oh-package.json5": `{name:'pkg',version:'1.0.0',types:'hidden.d.ets'}`,
		"/repo/oh_modules/pkg/hidden.d.ets":     `export declare function hidden(): void;`,
	}, true))
	options := &core.CompilerOptions{
		NoEmit:               core.TSTrue,
		Module:               core.ModuleKindESNext,
		ModuleResolution:     core.ModuleResolutionKindBundler,
		EtsAnnotationsEnable: core.TSTrue,
		PackageManagerType:   "ohpm",
		OhPackageExports:     map[string][]string{"pkg": {"/repo/oh_modules/pkg/public.d.ets"}},
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       []string{"/repo/entry.ets"},
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/repo", fs, bundled.LibPath(), nil, nil, nil),
	})
	diagnostics := program.GetSemanticDiagnostics(t.Context(), nil)
	if !slices.ContainsFunc(diagnostics, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 28045 }) {
		t.Fatalf("missing TS28045: %v", diagnostics)
	}
	if program.GetSourceFile("/repo/oh_modules/pkg/hidden.d.ets") != nil {
		t.Fatal("non-oh-export dependency was materialized into the program")
	}
}

func TestArkUIConfiguredFunctionDecoratorSkipsOrdinaryDecoratorSignature(t *testing.T) {
	t.Parallel()

	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/input.ets": `declare const Builder: number; @Builder function content() {}`,
	}, true))
	options := &core.CompilerOptions{
		NoEmit:                 core.TSTrue,
		Module:                 core.ModuleKindESNext,
		ModuleResolution:       core.ModuleResolutionKindBundler,
		ExperimentalDecorators: core.TSTrue,
		EtsAnnotationsEnable:   core.TSTrue,
		Ets:                    etstest.Options(),
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       []string{"/input.ets"},
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	input := program.GetSourceFile("/input.ets")
	if input == nil || !slices.ContainsFunc(input.Statements.Nodes, func(node *ast.Node) bool {
		return ast.IsFunctionDeclaration(node) && ast.HasDecorators(node)
	}) {
		t.Fatalf("configured ETS function decorator was not preserved in AST: %#v", input)
	}
	diagnostics := program.GetSemanticDiagnostics(t.Context(), nil)
	if slices.ContainsFunc(diagnostics, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 28004 }) {
		t.Fatalf("configured ArkTS function decorator used ordinary decorator checking: %v", diagnostics)
	}
}

func TestArkUIEmit(t *testing.T) {
	for _, declarationOnly := range []bool{true, false} {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
			"/Page.ets": `export struct Page { value: string = "hello"; build() {} }`,
		}, true))
		options := &core.CompilerOptions{Declaration: core.TSTrue, Module: core.ModuleKindESNext, Target: core.ScriptTargetESNext}
		if declarationOnly {
			options.EmitDeclarationOnly = core.TSTrue
		}
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/Page.ets"}, CompilerOptions: options}},
			Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		written := make(map[string]string)
		result := program.Emit(t.Context(), compiler.EmitOptions{WriteFile: func(name, text string, _ *compiler.WriteFileData) error { written[name] = text; return nil }})
		if declarationOnly && (result.EmitSkipped || len(result.Diagnostics) != 0) {
			t.Fatalf("declaration emit failed: %v", result.Diagnostics)
		}
		if !declarationOnly && (!result.EmitSkipped || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code() != 100069) {
			t.Fatalf("expected SDK emit diagnostic, got %v", result.Diagnostics)
		}
		if _, ok := written["/Page.js"]; ok {
			t.Fatal("emitted invalid ArkUI JavaScript")
		}
		if !strings.Contains(written["/Page.d.ets"], "struct Page") {
			t.Fatalf("struct lost in declaration emit: %v", written)
		}
	}
}

func TestArkUIModuleResolution(t *testing.T) {
	for _, specifier := range []string{"./model", "./model.ets", "./types", "./types.d.ets", "./folder"} {
		fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
			"/main.ets":         `import type { value } from "` + specifier + `"; export type Result = typeof value;`,
			"/model.ets":        `export const value: string = "hello";`,
			"/types.d.ets":      `export declare const value: string;`,
			"/folder/index.ets": `export const value: string = "hello";`,
		}, true))
		options := &core.CompilerOptions{NoEmit: core.TSTrue, AllowImportingTsExtensions: core.TSTrue, Module: core.ModuleKindESNext, ModuleResolution: core.ModuleResolutionKindBundler}
		program := compiler.NewProgram(compiler.ProgramOptions{
			Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/main.ets"}, CompilerOptions: options}},
			Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
		})
		if ds := program.GetSemanticDiagnostics(t.Context(), nil); len(ds) != 0 {
			t.Errorf("%s: %v", specifier, ds)
		}
	}
}
