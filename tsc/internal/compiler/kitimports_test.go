package compiler_test

import (
	"slices"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

const ohKitLoaderPath = "/sdk/openharmony/ets/build-tools/ets-loader"

// OH parser.ts::parseSourceFileWorker runs ohApi.ts::processKit before module
// collection and binding. The virtual imports therefore define the checked
// source graph while preserving local aliases, import type, and import lazy.
func TestOHKitImportsDefineSemanticProgram(t *testing.T) {
	files := map[string]string{
		"/entry.ets": "import lazy DefaultThing, { Foo as Local, type Bar } from '@kit.Test';\n" +
			"const value: number = new DefaultThing().value;\n" +
			"const local: Local | undefined = undefined;\n" +
			"let bar: Bar = {};\nvalue; local; bar;",
		"/sdk/openharmony/ets/build-tools/ets-loader/kit_configs/@kit.Test.json": "{\"symbols\":{\"default\":{\"source\":\"@ohos.default.d.ts\",\"bindings\":\"default\"},\"Foo\":{\"source\":\"@ohos.foo.d.ts\",\"bindings\":\"ExportedFoo\"},\"Bar\":{\"source\":\"@ohos.bar.d.ts\",\"bindings\":\"Bar\"}}}",
		"/sdk/api/@ohos.default.d.ts":                                            "export default class DefaultThing { value: number; }",
		"/sdk/api/@ohos.foo.d.ts":                                                "export declare class ExportedFoo {}",
		"/sdk/api/@ohos.bar.d.ts":                                                "export interface Bar {}",
	}
	fs := bundled.WrapFS(vfstest.FromMap(files, true))
	options := &core.CompilerOptions{
		NoEmit: core.TSTrue, Target: core.ScriptTargetESNext, Module: core.ModuleKindESNext,
		ModuleResolution: core.ModuleResolutionKindBundler, EtsLoaderPath: ohKitLoaderPath,
		OhSdkConfigs:    []core.OhSdkConfig{{ApiPaths: []string{"/sdk/api"}}},
		OhSystemModules: []string{"@ohos.default.d.ts", "@ohos.foo.d.ts", "@ohos.bar.d.ts"},
	}
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/entry.ets"}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if diagnostics := program.GetSyntacticDiagnostics(t.Context(), nil); len(diagnostics) != 0 {
		t.Fatalf("syntactic diagnostics: %v", diagnostics)
	}
	if diagnostics := program.GetSemanticDiagnostics(t.Context(), nil); len(diagnostics) != 0 {
		t.Fatalf("semantic diagnostics: %v", diagnostics)
	}
	entry := program.GetSourceFile("/entry.ets")
	if entry == nil || len(entry.MarkedKitImportRanges) != 1 {
		t.Fatalf("marked kit ranges = %v", entry)
	}
	imports := entry.Statements.Nodes[:3]
	wantModules := []string{"@ohos.default", "@ohos.foo", "@ohos.bar"}
	for i, statement := range imports {
		declaration := statement.AsImportDeclaration()
		clause := declaration.ImportClause.AsImportClause()
		if declaration.ModuleSpecifier.Text() != wantModules[i] || !clause.IsLazy || statement.Flags&ast.NodeFlagsKitImport == 0 {
			t.Fatalf("virtual import %d = %s, lazy=%v, flags=%v", i, declaration.ModuleSpecifier.Text(), clause.IsLazy, statement.Flags)
		}
	}
	if !imports[2].AsImportDeclaration().ImportClause.IsTypeOnly() {
		t.Fatal("specifier-level import type was not preserved on the virtual import")
	}
}

func TestOHKitImportFallsBackAsWholeDeclaration(t *testing.T) {
	fs := vfstest.FromMap(map[string]string{
		"/sdk/openharmony/ets/build-tools/ets-loader/kit_configs/@kit.Test.json": "{\"symbols\":{\"Good\":{\"source\":\"@ohos.good.d.ts\",\"bindings\":\"Good\"}}}",
	}, true)
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{
		FileName: "/entry.ets", Path: "/entry.ets", EtsLoaderPath: ohKitLoaderPath,
	}, "import { Good, Missing } from '@kit.Test';", core.ScriptKindETS)
	processor := compiler.KitImportProcessor{}
	if processor.Process(file, fs) {
		t.Fatal("partially valid kit import must remain unchanged")
	}
	if got := file.Statements.Nodes[0].AsImportDeclaration().ModuleSpecifier.Text(); got != "@kit.Test" {
		t.Fatalf("module specifier = %q", got)
	}
	if len(file.MarkedKitImportRanges) != 0 {
		t.Fatalf("fallback import was marked transformed: %v", file.MarkedKitImportRanges)
	}
}

func TestOHKitAddsComponentAttributeBindings(t *testing.T) {
	fs := vfstest.FromMap(map[string]string{
		"/sdk/openharmony/ets/build-tools/ets-loader/kit_configs/@kit.ArkUI.json": "{\"symbols\":{\"ArcList\":{\"source\":\"@ohos.arkui.ArcList.d.ts\",\"bindings\":\"ArcList\"},\"ArcListAttribute\":{\"source\":\"@ohos.arkui.ArcList.d.ts\",\"bindings\":\"ArcListAttribute\"}}}",
	}, true)
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{
		FileName: "/entry.ets", Path: "/entry.ets", EtsLoaderPath: ohKitLoaderPath,
	}, "import { ArcList } from '@kit.ArkUI'; import { ArcSwiper } from '@ohos.arkui.ArcSwiper';", core.ScriptKindETS)
	processor := compiler.KitImportProcessor{}
	if !processor.Process(file, fs) {
		t.Fatal("kit import was not transformed")
	}
	var names []string
	for _, statement := range file.Statements.Nodes {
		clause := statement.AsImportDeclaration().ImportClause.AsImportClause()
		for _, element := range clause.NamedBindings.AsNamedImports().Elements.Nodes {
			names = append(names, element.Name().Text())
		}
	}
	for _, want := range []string{"ArcList", "ArcListAttribute", "ArcSwiper", "ArcSwiperAttribute"} {
		if !slices.Contains(names, want) {
			t.Fatalf("supplemented names = %v, missing %s", names, want)
		}
	}
}

func TestOHKitTransformCanBeDisabled(t *testing.T) {
	fs := vfstest.FromMap(map[string]string{}, true)
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{
		FileName: "/entry.ets", Path: "/entry.ets", EtsLoaderPath: ohKitLoaderPath, NoTransformedKitInParser: true,
	}, "import { Value } from '@kit.Test';", core.ScriptKindETS)
	processor := compiler.KitImportProcessor{}
	if processor.Process(file, fs) {
		t.Fatal("noTransformedKitInParser was ignored")
	}
}
