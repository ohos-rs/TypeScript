package compiler_test

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
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
		{"binding", `Card({ title: $message }); Text($$this.message); Text($r("app.string.title"))`, false},
		{"unknown binding", `Text($missing)`, true},
		{"generic component", `Box<string>({ value: "hello" })`, false},
		{"generic component error", `Box<string>({ value: 42 })`, true},
		{"required property missing", `Box<string>()`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
				"/tsconfig.json": `{"compilerOptions":{"noEmit":true,"lib":["es2020"],"strict":true,"module":"esnext","moduleResolution":"bundler","allowImportingTsExtensions":true},"include":["**/*"]}`,
				"/sdk.d.ets":     `declare class CommonAttribute { width(value: number): this; } declare class TextAttribute extends CommonAttribute { fontSize(value: number): this; stateStyles(styles: { normal?: TextAttribute }): this; } declare function Text(value: string): TextAttribute; declare function Column(): CommonAttribute; declare function $r(name: string): string;`,
				"/Card.ets":      `@Component export struct Box<T> { @Require @Prop value: T; build() {} } @Component export struct Card { @Prop title: string; build() { Text(this.title) } }`,
				"/Page.ets":      `import { Card, Box } from "./Card"; @Styles function common() { .width(100) } @Extend(Text) function emphasis(size: number) { .fontSize(size) } @Entry @Component struct Page { @State message: string = "hello"; build() { ` + tc.expression + ` } }`,
			}, true))
			host := compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil)
			config, errors := tsoptions.GetParsedCommandLineOfConfigFile("/tsconfig.json", &core.CompilerOptions{}, nil, host, nil)
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
