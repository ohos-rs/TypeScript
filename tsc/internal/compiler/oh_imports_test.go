package compiler_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

// OH checker.ts::resolveExternalModule: .so is a substring test, performed
// before ambient lookup except for ETS strict linter mode. Missing .so modules
// still warn after that lookup; tsImportSoCheck enables ordinary TS errors.
func TestOHSoImports(t *testing.T) {
	for _, tc := range []struct {
		name, file, specifier              string
		ambient, enabled, lint, compatible bool
		code                               int
	}{
		{"TS default", "/entry.ts", "lib.so", false, false, false, false, 28014},
		{"ETS default", "/entry.ets", "lib.so", false, false, false, false, 28014},
		{"substring", "/entry.ts", "lib.so.extra", false, false, false, false, 28014},
		{"ambient bypass", "/entry.ts", "lib.so", true, false, false, false, 28014},
		{"enabled ambient", "/entry.ts", "lib.so", true, true, false, false, 0},
		{"enabled missing", "/entry.ts", "lib.so", false, true, false, false, 2307},
		{"strict ETS ambient", "/entry.ets", "lib.so", true, false, true, false, 0},
		{"strict ETS missing", "/entry.ets", "lib.so", false, false, true, false, 28014},
		{"compatible ETS ambient", "/entry.ets", "lib.so", true, false, true, true, 28014},
		{"strict TS ambient", "/entry.ts", "lib.so", true, false, true, false, 28014},
		{"normal missing", "/entry.ts", "native", false, false, false, false, 2307},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{tc.file: "import { value } from '" + tc.specifier + "'; value;"}
			roots := []string{tc.file}
			if tc.ambient {
				files["/types.d.ts"] = "declare module '" + tc.specifier + "' { export const value: number; }"
				roots = append(roots, "/types.d.ts")
			}
			checkOHImport(t, files, roots, &core.CompilerOptions{
				NoEmit: core.TSTrue, TsImportSoCheck: core.BoolToTristate(tc.enabled),
				NeedDoArkTsLinter: core.BoolToTristate(tc.lint), IsCompatibleVersion: core.BoolToTristate(tc.compatible),
			}, tc.code)
		})
	}
}

// OH's Sendable exception permits only static imports/exports from source TS
// or SDK .d.ts; it does not exempt dynamic import, import types, external .d.ts,
// or TS import-equals. JS/TSX are not ScriptKind.TS in this source branch.
func TestOHTsImportEts(t *testing.T) {
	for _, tc := range []struct {
		name, file, body           string
		lint, compatible, sendable bool
		code                       int
	}{
		{"disabled", "/entry.ts", "import { value } from '/target.ets';", false, false, false, 0},
		{"strict", "/entry.ts", "import { value } from '/target.ets';", true, false, false, 28017},
		{"compatible", "/entry.ts", "import { value } from '/target.ets';", true, true, false, 28016},
		{"sendable import", "/entry.ts", "import { value } from '/target.ets';", true, false, true, 0},
		{"sendable export", "/entry.ts", "export { value } from '/target.ets';", true, false, true, 0},
		{"dynamic", "/entry.ts", "import('/target.ets');", true, false, true, 28017},
		{"import type", "/entry.ts", "type T = typeof import('/target.ets');", true, false, true, 28017},
		{"import equals", "/entry.ts", "import target = require('/target.ets');", true, false, true, 28017},
		{"external declaration", "/app/entry.d.ts", "import { value } from '/target.ets';", true, false, true, 28017},
		{"SDK declaration", "/sdk/ets/api/entry.d.ts", "import { value } from '/target.ets';", true, false, false, 0},
		{"SDK prefix", "/sdk/ets-other/entry.d.ts", "import { value } from '/target.ets';", true, false, false, 0},
		{"SDK source", "/sdk/ets/api/entry.ts", "import { value } from '/target.ets';", true, false, false, 28017},
		{"ETS source", "/entry.ets", "import { value } from '/target.ets';", true, false, false, 0},
		{"TSX source", "/entry.tsx", "import { value } from '/target.ets';", true, false, false, 0},
		{"JS source", "/entry.js", "import { value } from '/target.ets';", true, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkOHImport(t, map[string]string{tc.file: tc.body, "/target.ets": "export const value = 1;"}, []string{tc.file}, &core.CompilerOptions{
				NoEmit: core.TSTrue, AllowImportingTsExtensions: core.TSTrue, AllowJs: core.TSTrue, CheckJs: core.TSTrue,
				Target: core.ScriptTargetESNext, Module: core.ModuleKindCommonJS, Jsx: core.JsxEmitPreserve,
				EtsLoaderPath: "/sdk/ets/build-tools/ets-loader", NeedDoArkTsLinter: core.BoolToTristate(tc.lint),
				IsCompatibleVersion: core.BoolToTristate(tc.compatible), TsImportSendableEnable: core.BoolToTristate(tc.sendable),
			}, tc.code)
		})
	}
}

func checkOHImport(t *testing.T, files map[string]string, roots []string, options *core.CompilerOptions, code int) {
	t.Helper()
	fs := bundled.WrapFS(vfstest.FromMap(files, true))
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: roots, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
		t.Fatal(ds)
	}
	ds := program.GetSemanticDiagnostics(t.Context(), nil)
	if code == 0 {
		if len(ds) != 0 {
			t.Fatal(ds)
		}
		return
	}
	category := diagnostics.CategoryError
	if code == 28014 || code == 28016 {
		category = diagnostics.CategoryWarning
	}
	if len(ds) != 1 || ds[0].Code() != int32(code) || ds[0].Category() != category {
		t.Fatalf("expected %d (%v), got %v", code, category, ds)
	}
}
