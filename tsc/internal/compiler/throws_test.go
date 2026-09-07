package compiler_test

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

// OH checker.ts::checkThrowableFunction/isThrowsHandled: notably arrows are
// not reporting owners, and a try/finally without catch does not handle throws.
func TestOHThrowsCallContexts(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		count      int
	}{
		{"unhandled", "function f() { risky(); }", 1},
		{"top level", "risky();", 0},
		{"arrow", "const f = () => risky();", 0},
		{"function expression", "const f = function() { risky(); };", 0},
		{"propagated", "/** @throws failure */ function f() { risky(); }", 0},
		{"caught", "function f() { try { risky(); } catch {} }", 0},
		{"finally only", "function f() { try { risky(); } finally {} }", 1},
		{"in catch", "function f() { try {} catch { risky(); } }", 1},
		{"in finally", "function f() { try {} catch {} finally { risky(); } }", 1},
		{"nested declaration", "function f() { try { function g() { risky(); } } catch {} }", 1},
		{"catch chain", "function f() { risky().catch(() => {}); }", 0},
		{"method", "class C { f() { risky(); } }", 1},
		{"getter", "class C { get f() { return risky(); } }", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := "/** @throws failure */ declare function risky(): { catch(cb: () => void): void };\n" + tc.body
			checkOHThrows(t, map[string]string{"/entry.ets": source}, "/entry.ets", &core.CompilerOptions{NoEmit: core.TSTrue}, tc.count)
		})
	}
}

func TestOHThrowsSDKAndTags(t *testing.T) {
	version := 18.0
	for _, tc := range []struct {
		name, path, tag, params string
		count                   int
	}{
		{"plain", "/sdk/api.d.ts", "failure", "", 1},
		{"SDK 401", "/sdk/api.d.ts", "{ BusinessError } 401 - invalid", "", 0},
		{"SDK exact spacing", "/sdk/api.d.ts", "{BusinessError} 401 - invalid", "", 1},
		{"SDK missing gap", "/sdk/api.d.ts", "{ BusinessError }401 - invalid", "", 1},
		{"SDK double gap", "/sdk/api.d.ts", "{ BusinessError }  401 - invalid", "", 1},
		{"unknown tag prose", "/sdk/api.d.ts", "{ this is prose, not a type } failure", "", 1},
		{"non SDK 401", "/app/api.d.ts", "{ BusinessError } 401 - invalid", "", 1},
		{"node modules", "/node_modules/api.d.ts", "failure", "", 0},
		{"oh modules", "/oh_modules/api.d.ts", "failure", "", 0},
		{"JS utility", "/js_util_module/api.d.ts", "failure", "", 0},
		{"async callback", "/sdk/api.d.ts", "failure", "cb?: AsyncCallback", 0},
		{"error callback", "/sdk/api.d.ts", "failure", "cb?: ErrorCallback", 0},
		{"simple future", "/sdk/api.d.ts", "failure [since 19]", "", 0},
		{"simple current", "/sdk/api.d.ts", "failure [since 18]", "", 1},
		{"closed range", "/sdk/api.d.ts", "failure [since 16 - 17]", "", 0},
		{"reversed range", "/sdk/api.d.ts", "failure [since 20 - 18]", "", 1},
		{"parenthesized", "/sdk/api.d.ts", "failure [since 5.0.0(19)]", "", 0},
		{"parenthesized range", "/sdk/api.d.ts", "failure [since 5.0.0(17) - 6.0.0(18)]", "", 1},
		{"mixed parenthesized", "/sdk/api.d.ts", "failure [since 5.0.0(17) - 19.0.0]", "", 1},
		{"mixed numeric", "/sdk/api.d.ts", "failure [since 19 - 20.0.0]", "", 0},
		{"version", "/sdk/api.d.ts", "failure [since 19.0.0]", "", 0},
		{"version range", "/sdk/api.d.ts", "failure [since 17.0.0 - 18.0.0]", "", 1},
		{"non SDK version", "/app/api.d.ts", "failure [since 99]", "", 1},
		{"nonmatching format", "/sdk/api.d.ts", "failure [Since 99]", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkOHThrows(t, map[string]string{
				"/entry.ts": "import { risky } from '" + strings.TrimSuffix(tc.path, ".d.ts") + "'; function f() { risky(); }",
				tc.path:     "type AsyncCallback = () => void; type ErrorCallback = () => void;\n/** @throws " + tc.tag + " */\nexport declare function risky(" + tc.params + "): void;",
			}, "/entry.ts", &core.CompilerOptions{NoEmit: core.TSTrue, EtsLoaderPath: "/sdk/openharmony/ets/build-tools/ets-loader", CompileSdkVersion: &version}, tc.count)
		})
	}
}

func TestOHThrowsAbsentSDKVersion(t *testing.T) {
	checkOHThrows(t, map[string]string{
		"/entry.ts": "/** @throws failure [since 999] */ declare function risky(): void; function f() { risky(); }",
	}, "/entry.ts", &core.CompilerOptions{NoEmit: core.TSTrue, EtsLoaderPath: "/sdk/openharmony/ets/build-tools/ets-loader"}, 1)
}

func checkOHThrows(t *testing.T, files map[string]string, root string, options *core.CompilerOptions, count int) {
	t.Helper()
	fs := bundled.WrapFS(vfstest.FromMap(files, true))
	program := compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{FileNames: []string{root}, CompilerOptions: options}},
		Host:   compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
	if ds := program.GetSyntacticDiagnostics(t.Context(), nil); len(ds) != 0 {
		t.Fatal(ds)
	}
	ds := program.GetSemanticDiagnostics(t.Context(), nil)
	if len(ds) != count {
		t.Fatalf("expected %d warnings, got %v", count, ds)
	}
	for _, d := range ds {
		if d.Code() != 28040 || d.Category() != diagnostics.CategoryWarning {
			t.Fatal(d)
		}
	}
}
