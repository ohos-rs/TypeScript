package compiler_test

import (
	"encoding/json"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

func TestEtsLibrariesLoadAndVisibility(t *testing.T) {
	// OH program.ts library loading and utilities.ts::getEtsLibs: explicit
	// library paths, one-level references, merged declarations and TS isolation.
	var ets core.EtsOptions
	if err := json.Unmarshal([]byte(`{"libs":["./sdk/components.d.ts"]}`), &ets); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"/sdk/components.d.ts": "/// <reference path=\"./direct.d.ts\" />\ndeclare const RootWidget: number; interface SharedWidget { root: number; }",
		"/sdk/direct.d.ts":     "/// <reference path=\"./indirect.d.ts\" />\ndeclare const DirectWidget: number; declare const inside: typeof RootWidget;",
		"/sdk/indirect.d.ts":   "declare const IndirectWidget: number;",
		"/shared.d.ts":         "interface SharedWidget { other: number; }",
		"/input.ets":           "export {}; RootWidget; DirectWidget; IndirectWidget; let shared: SharedWidget;",
		"/input.ts":            "export {}; RootWidget; DirectWidget; IndirectWidget; let shared: SharedWidget;",
	}
	options := &core.CompilerOptions{Ets: ets, Lib: []string{"lib.es2020.d.ts"}, NoEmit: core.TSTrue}
	program := compiler.NewProgram(compiler.ProgramOptions{Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/input.ets", "/input.ts", "/shared.d.ts"}, CompilerOptions: options}}, Host: compiler.NewCompilerHost("/", bundled.WrapFS(vfstest.FromMap(files, true)), bundled.LibPath(), nil, nil, nil)})
	lib := program.GetSourceFile("/sdk/components.d.ts")
	if lib == nil || !program.IsSourceFileDefaultLibrary(lib.Path()) {
		t.Fatal("configured ETS library must be loaded as a default library")
	}
	for _, file := range []string{"/input.ets", "/sdk/components.d.ts", "/sdk/direct.d.ts", "/shared.d.ts"} {
		if ds := program.GetSemanticDiagnostics(t.Context(), program.GetSourceFile(file)); len(ds) != 0 {
			t.Fatalf("%s: %v", file, ds)
		}
	}
	ds := program.GetSemanticDiagnostics(t.Context(), program.GetSourceFile("/input.ts"))
	if len(ds) != 2 {
		t.Fatalf("only root/direct ETS declarations must be hidden from TS: %v", ds)
	}
	for _, d := range ds {
		if d.Code() != 2304 && d.Code() != 2552 {
			t.Fatalf("unexpected TS diagnostic: %v", d)
		}
	}
	for _, tc := range []struct {
		name  string
		lib   []string
		noLib core.Tristate
	}{
		{"implicit standard library", nil, core.TSFalse},
		{"noLib", []string{"lib.es2020.d.ts"}, core.TSTrue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := *options
			opts.Lib, opts.NoLib = tc.lib, tc.noLib
			p := compiler.NewProgram(compiler.ProgramOptions{Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{"/input.ets"}, CompilerOptions: &opts}}, Host: compiler.NewCompilerHost("/", bundled.WrapFS(vfstest.FromMap(files, true)), bundled.LibPath(), nil, nil, nil)})
			if p.GetSourceFile("/sdk/components.d.ts") != nil {
				t.Fatal("ets.libs must not bypass the source-owned library-loading branch")
			}
		})
	}
}
