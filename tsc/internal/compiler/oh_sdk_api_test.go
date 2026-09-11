package compiler_test

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/diagnosticwriter"
	"github.com/microsoft/TypeScript/tsc/internal/locale"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

func TestOHApiAvailableUsesSDKDeclarationIdentity(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.deviceInfo.d.ts": `declare namespace deviceInfo { function apiAvailable(version: string | number | null | undefined): boolean; }`,
		"/input.ets": `
            deviceInfo.apiAvailable('25.0.0');
            deviceInfo.apiAvailable('26.0.0');
            const local = { apiAvailable(_value: null): boolean { return true; } };
            local.apiAvailable(null);
        `,
	}, true))
	program := newOHSDKProgram(fs, []string{"/sdk/@ohos.deviceInfo.d.ts", "/input.ets"})
	diags := program.GetSemanticDiagnostics(t.Context(), nil)
	var sdk []*ast.Diagnostic
	for _, diagnostic := range diags {
		if diagnostic.Code() == 28007 {
			sdk = append(sdk, diagnostic)
		}
	}
	if len(sdk) != 1 || sdk[0].Category() != diagnostics.CategoryError || !strings.Contains(ohDiagnosticText(sdk[0]), "M must be greater than or equal to 26") {
		t.Fatalf("apiAvailable SDK diagnostics = %#v", sdk)
	}
}

func TestArkTSSemanticsMatchOHNullishCoalescingDiagnostics(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/input.ets": `const value = (1 + 2) ?? 0;`,
		"/input.ts":  `const value = (1 + 2) ?? 0;`,
	}, true))
	arktsOptions := &core.CompilerOptions{
		NoEmit:        core.TSTrue,
		EtsLoaderPath: "/sdk/ets/build-tools/ets-loader",
		Ets:           etstest.Options(),
	}
	arktsProgram := newOHSDKProgramWithOptions(fs, []string{"/input.ets"}, arktsOptions)
	for _, diagnostic := range arktsProgram.GetSemanticDiagnostics(t.Context(), nil) {
		if diagnostic.Code() == 2869 || diagnostic.Code() == 2871 {
			t.Fatalf("OH ArkTS mode reported newer TypeScript nullish diagnostic: %s", ohDiagnosticText(diagnostic))
		}
	}

	typescriptProgram := newOHSDKProgramWithOptions(fs, []string{"/input.ts"}, &core.CompilerOptions{NoEmit: core.TSTrue})
	found := false
	for _, diagnostic := range typescriptProgram.GetSemanticDiagnostics(t.Context(), nil) {
		found = found || diagnostic.Code() == 2869
	}
	if !found {
		t.Fatal("ordinary TypeScript mode must retain TS2869")
	}
}

// api_check_utils.ts::findNonNullType permits a nullable apiAvailable type
// when null/undefined removal leaves one constituent, and rejects ambiguous
// unions with multiple non-null constituents.
func TestOHApiAvailableFiltersNullableUnionBeforeIdentityCheck(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.deviceInfo.d.ts": `
			declare function apiAvailable(version: string | number): boolean;
			declare const nullableApi: typeof apiAvailable | undefined;
			declare const ambiguousApi: typeof apiAvailable | ((version: string | number) => number) | undefined;
		`,
		"/input.ets": `
			nullableApi('25.0.0');
			ambiguousApi('25.0.0');
		`,
	}, true))
	program := newOHSDKProgram(fs, []string{"/sdk/@ohos.deviceInfo.d.ts", "/input.ets"})
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "M must be greater than or equal to 26") {
		t.Fatalf("nullable apiAvailable diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

func TestOHAvailableContentAndParentVersion(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.annotation.d.ets": `@interface Available { minApiVersion: string }`,
		"/input.ets": `
            @Available({ minApiVersion: "25.0.0" })
            class InvalidFormat {}
            @Available({ minApiVersion: "26.0.0" })
            class Parent {
                @Available({ minApiVersion: "25" })
                child(): void {}
            }
        `,
	}, true))
	program := newOHSDKProgram(fs, []string{"/sdk/@ohos.annotation.d.ets", "/input.ets"})
	diags := program.GetSemanticDiagnostics(t.Context(), nil)
	var sdk []*ast.Diagnostic
	for _, diagnostic := range diags {
		if diagnostic.Code() == 28007 {
			sdk = append(sdk, diagnostic)
		}
	}
	if len(sdk) != 2 {
		t.Fatalf("available SDK diagnostics = %#v", sdk)
	}
	if !slices.ContainsFunc(sdk, func(diagnostic *ast.Diagnostic) bool {
		return diagnostic.Category() == diagnostics.CategoryError && strings.Contains(ohDiagnosticText(diagnostic), "11706016#")
	}) {
		t.Fatalf("missing invalid format diagnostic: %#v", sdk)
	}
	if !slices.ContainsFunc(sdk, func(diagnostic *ast.Diagnostic) bool {
		return diagnostic.Category() == diagnostics.CategoryWarning && strings.Contains(ohDiagnosticText(diagnostic), "outer annotation")
	}) {
		t.Fatalf("missing parent-version warning: %#v", sdk)
	}
}

// api_check_utils.ts::getJsDocNodeCheckConfig and expressionCheckByJsDoc
// attach SDK JSDoc contracts to the resolved declaration, not to matching
// member text in user code.
func TestOHSDKJSDocChecksResolvedDeclarations(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `
            /** @syscap SystemCapability.Base */ declare namespace sample {
              /**
               * @since 12
               * @syscap SystemCapability.Base
               */ function future(): void;
              /**
               * @permission ohos.permission.CAMERA
               * @syscap SystemCapability.Base
               */ function camera(): void;
              /** @syscap SystemCapability.Camera */ function cameraCap(): void;
              /**
               * @systemapi
               * @syscap SystemCapability.Base
               */ function systemOnly(): void;
              /**
               * @deprecated
               * @syscap SystemCapability.Base
               */ function old(): void;
              /**
               * @test
               * @syscap SystemCapability.Base
               */ function testOnly(): void;
              /**
               * @famodelonly
               * @syscap SystemCapability.Base
               */ function faOnly(): void;
            }
        `,
		"/project/entry/src/main/ets/entry.ets": `
            sample.future();
            sample.camera();
            sample.cameraCap();
            sample.systemOnly();
            sample.old();
            sample.testOnly();
            sample.faOnly();
        `,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	for _, expected := range []string{
		"supported since SDK version 12",
		"ohos.permission.CAMERA",
		"not supported on all devices",
		"is system api",
		"has been deprecated",
		"can only be used for testing directories",
		"11706008#",
	} {
		if !slices.ContainsFunc(diagnostics, func(diagnostic *ast.Diagnostic) bool {
			return strings.Contains(ohDiagnosticText(diagnostic), expected)
		}) {
			t.Fatalf("missing %q in SDK diagnostics: %v", expected, diagnosticTexts(diagnostics))
		}
	}
	if len(diagnostics) != 7 {
		t.Fatalf("SDK diagnostics = %v, want 7", diagnosticTexts(diagnostics))
	}
}

// SinceWarningSuppressor composes try/catch, undefined checks, @Available,
// SDK-version guards and @SuppressWarnings. All are source-owned suppressions,
// so they must prevent the carrier TS28007 diagnostic rather than filtering it
// after publication.
func TestOHSDKJSDocSuppressors(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.deviceInfo.d.ts": `
            /** @syscap SystemCapability.Base */ declare namespace deviceInfo {
              /** @syscap SystemCapability.Base */ const sdkApiVersion: number;
              /** @syscap SystemCapability.Base */ function apiAvailable(version: string | number): boolean;
            }
        `,
		"/sdk/@ohos.sample.d.ts": `/** @syscap SystemCapability.Base */ declare namespace sample {
            /**
             * @since 12
             * @syscap SystemCapability.Base
             */ function future(): void;
        }`,
		"/project/entry/src/main/ets/entry.ets": `
            try { sample.future(); } catch (_) {}
            if (sample.future !== undefined) { sample.future(); }
            if (deviceInfo.sdkApiVersion >= 12) { sample.future(); }
            const sdkGuard = "12";
            if (deviceInfo.sdkApiVersion >= sdkGuard) { sample.future(); }
            const { bindingGuard = "12" }: { bindingGuard?: string } = {};
            if (deviceInfo.sdkApiVersion >= bindingGuard) { sample.future(); }
            if (deviceInfo.apiAvailable(12)) { sample.future(); }
            @Available({ minApiVersion: "12" })
            class Guarded { run(): void { sample.future(); } }
            @SuppressWarnings({ rules: [SuppressWarningsType.COMPATIBILITY] })
            class Suppressed { run(): void { sample.future(); } }
        `,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.deviceInfo.d.ts", "/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("suppressed SDK diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// SdkComparisonHelper::isApiAvailableHelper identifies the guard from its
// expression text and validated literal; unlike the standalone apiAvailable
// diagnostic, this suppressor does not verify the declaration's SDK identity.
func TestOHSDKApiAvailableSuppressorMatchesSourceTextBehavior(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 12
             * @syscap SystemCapability.Base
             */ declare function future(): void;`,
		"/project/entry/src/main/ets/entry.ets": `
            const deviceInfo = 0;
            const local = { apiAvailable(_value: number): boolean { return true; } };
            if (local.apiAvailable(12)) { future(); }
        `,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("text-matched apiAvailable diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// SdkComparisonHelper rejects sdkApiVersion suppression for an M.S.F API
// requirement whose major version is greater than 26.
func TestOHSDKVersionGuardDoesNotSuppressPost26MSFRequirement(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.deviceInfo.d.ts": `
            /** @syscap SystemCapability.Base */ declare namespace deviceInfo {
              /** @syscap SystemCapability.Base */ const sdkApiVersion: number;
            }
        `,
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 27.0.0
             * @syscap SystemCapability.Base
             */ declare function future(): void;`,
		"/project/entry/src/main/ets/entry.ets": `
            if (deviceInfo.sdkApiVersion >= 27) { future(); }
        `,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.deviceInfo.d.ts", "/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "27.0.0") {
		t.Fatalf("post-26 M.S.F diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// getAvailableCheckConfig deliberately uses `since` as its required tag. The
// callback may detect an incompatible @Available, but checker.ts does not emit
// when the same declaration already has @since.
func TestOHSDKProjectAvailableWithSinceDoesNotReport(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/project/library.ets": `
            /** @since 12 */
            @Available({ minApiVersion: "12" })
            export function future(): void {}
        `,
		"/project/entry/src/main/ets/entry.ets": `import { future } from "/project/library.ets"; future();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	program := newOHSDKProgramWithOptions(fs, []string{"/project/library.ets", "/project/entry/src/main/ets/entry.ets"}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("Available-plus-since diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_utils.ts::checkSyscapAbility initializes the declaration's syscap
// value to the empty string. With a configured intersection set, an SDK API
// without @syscap is therefore unsupported and must warn.
func TestOHSDKMissingSyscapWarns(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts":                `declare function plain(): void;`,
		"/project/entry/src/main/ets/entry.ets": `plain();`,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "not supported on all devices") {
		t.Fatalf("missing-syscap diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_utils.ts::checkSyscapAbility invokes class-style apiCheckPlugins
// only after the built-in intersection accepts the declaration syscap.
func TestOHSDKExternalSyscapClassChecker(t *testing.T) {
	t.Parallel()
	compatible := float64(12)
	compile := float64(12)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 11
             * @syscap SystemCapability.Base
             */ declare function current(): void;`,
		"/project/entry/src/main/ets/entry.ets": `current();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhSdkClassCheckPlugins = []core.OhSdkClassCheckPlugin{{
		TagName: "syscap", Path: "/sdk/syscap.js", ClassName: "SyscapChecker",
	}}
	options.OhSdkPluginProjectConfig = map[string]any{
		"customPluginValue":     "preserved",
		"syscapIntersectionSet": []string{"SystemCapability.Base"},
		"strictMode": map[string]any{
			"apiCompatibilityCheck": "warning",
		},
	}
	options.SetOhSdkPluginExecutor(syscapSDKPluginExecutor{})
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "extension device excludes current") {
		t.Fatalf("external syscap diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// utilities.ts::getJSDocTags reads only the last JSDoc block at the nearest
// owning location. SDK checks must not merge tags from an earlier adjacent
// block into the declaration contract.
func TestOHSDKUsesLastJSDocBlockAtNearestLocation(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `
            /** @since 12 */
            /** @syscap SystemCapability.Base */
            declare function current(): void;
        `,
		"/project/entry/src/main/ets/entry.ets": `current();`,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("adjacent-JSDoc diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// SinceVersionChecker takes originCompatibleSdkVersion.toString() before the
// normalized compatibleSdkVersion supplied by the build configuration.
func TestOHSDKOriginCompatibleVersionTakesPrecedence(t *testing.T) {
	t.Parallel()
	compatible := float64(12)
	compile := float64(12)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 11
             * @syscap SystemCapability.Base
             */ declare function future(): void;`,
		"/project/entry/src/main/ets/entry.ets": `future();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhOriginCompatibleSdkVersion = "10"
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "current compatible SDK version is 10") {
		t.Fatalf("origin-compatible diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// main.js::collectExternalApiCheckPlugin and
// api_check_utils.ts::initValueChecker select the runtime-specific SDK
// CompatibilityCheck before comparing the original compatible SDK string.
func TestOHSDKExternalSinceCompatibilityChecker(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 11
             * @syscap SystemCapability.Base
             */ declare function current(): void;`,
		"/project/entry/src/main/ets/entry.ets": `current();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhOriginCompatibleSdkVersion = "5.0.5(17)"
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{{
		OSName: "HarmonyOS", Tag: "since", Type: "CompatibilityCheck",
		Path: "/sdk/since-checker.js", FunctionName: "checkSinceValue",
	}}
	options.SetOhSdkPluginExecutor(harmonySDKPluginExecutor{})
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("external since-checker diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// third_party_typescript checker.ts::createTypeChecker disables SDK JSDoc
// callbacks on the second strict checker in a normal ArkTS linter build. The
// complete build diagnostic sequence must invoke the SDK checker only from the
// first, non-strict semantic pass.
func TestOHSDKExternalSinceCompatibilityCheckerRunsOnceBeforeArkTSLinter(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 11
             * @syscap SystemCapability.Base
             */ declare function current(): void;`,
		"/project/entry/src/main/ets/entry.ets": `current();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.NeedDoArkTsLinter = core.TSTrue
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhOriginCompatibleSdkVersion = "5.0.5(17)"
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{{
		OSName: "HarmonyOS", Tag: "since", Type: "CompatibilityCheck",
		Path: "/sdk/since-checker.js", FunctionName: "checkSinceValue",
	}}
	executor := &countingHarmonySDKPluginExecutor{}
	options.SetOhSdkPluginExecutor(executor)
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	diagnostics := program.GetArkTSBuildDiagnostics(t.Context())
	if sdkDiagnostics := ohSDKDiagnostics(append(diagnostics.Semantic, diagnostics.Linter...)); len(sdkDiagnostics) != 0 {
		t.Fatalf("ArkTS build external since-checker diagnostics = %v", diagnosticTexts(sdkDiagnostics))
	}
	if calls := executor.valueCalls.Load(); calls != 1 {
		t.Fatalf("external since-checker calls = %d, want 1", calls)
	}
}

// checker.ts keeps getJsDocNodeCheckedConfig on the strict checker when
// strictCheckerOnly is enabled. In that mode the ordinary ETS semantic pass is
// skipped and the one SDK invocation belongs to the strict semantic pass.
func TestOHSDKExternalSinceCompatibilityCheckerRunsInStrictCheckerOnly(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 11
             * @syscap SystemCapability.Base
             */ declare function current(): void;`,
		"/project/entry/src/main/ets/entry.ets": `current();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.NeedDoArkTsLinter = core.TSTrue
	options.StrictCheckerOnly = core.TSTrue
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhOriginCompatibleSdkVersion = "5.0.5(17)"
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{{
		OSName: "HarmonyOS", Tag: "since", Type: "CompatibilityCheck",
		Path: "/sdk/since-checker.js", FunctionName: "checkSinceValue",
	}}
	executor := &countingHarmonySDKPluginExecutor{}
	options.SetOhSdkPluginExecutor(executor)
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	diagnostics := program.GetArkTSBuildDiagnostics(t.Context())
	if sdkDiagnostics := ohSDKDiagnostics(append(diagnostics.Semantic, diagnostics.Linter...)); len(sdkDiagnostics) != 0 {
		t.Fatalf("strict-only external since-checker diagnostics = %v", diagnosticTexts(sdkDiagnostics))
	}
	if calls := executor.valueCalls.Load(); calls != 1 {
		t.Fatalf("strict-only external since-checker calls = %d, want 1", calls)
	}
}

// checkFormatResult and validateApiAvailableArgument delegate HarmonyOS
// annotation/distribution formats to the functions registered from the SDK.
func TestOHSDKExternalFormatAndDistributionCheckers(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.annotation.d.ets": `@interface Available { minApiVersion: string }`,
		"/sdk/@ohos.deviceInfo.d.ts":  `declare namespace deviceInfo { function apiAvailable(version: string | number): boolean; }`,
		"/project/entry/src/main/ets/entry.ets": `
            @Available({ minApiVersion: "HarmonyOS 5.0.5(17)" })
            class Current {}
            deviceInfo.apiAvailable("5.0.5(17)");
        `,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhAllModulePaths = append(options.OhAllModulePaths, "/sdk/@ohos.annotation.d.ets")
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{
		{OSName: "HarmonyOS", Tag: "available", Type: "FormatValidation", Path: "/sdk/format.js", FunctionName: "checkAvailableFormat"},
		{OSName: "HarmonyOS", Tag: "since", Path: "/sdk/distribution.js", FunctionName: "checkFirstDistribution"},
		{OSName: "HarmonyOS", Tag: "since", Path: "/sdk/distribution.js", FunctionName: "checkLastDistribution"},
	}
	options.SetOhSdkPluginExecutor(harmonySDKPluginExecutor{})
	program := newOHSDKProgramWithOptions(fs, []string{
		"/sdk/@ohos.annotation.d.ets", "/sdk/@ohos.deviceInfo.d.ts", "/project/entry/src/main/ets/entry.ets",
	}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("external format/distribution diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_utils.ts::isCheckDistributionOSVersion returns the value retained
// from an earlier callback when loading or invoking a later registration fails.
func TestOHSDKDistributionRetainsValueBeforeFailure(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.deviceInfo.d.ts":            `declare namespace deviceInfo { function apiAvailable(version: string | number): boolean; }`,
		"/project/entry/src/main/ets/entry.ets": `deviceInfo.apiAvailable("5.0.5(17)");`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{
		{OSName: "HarmonyOS", Tag: "since", Path: "/sdk/distribution.js", FunctionName: "checkDistribution"},
		{OSName: "HarmonyOS", Tag: "since", Path: "/sdk/missing.js", FunctionName: "loadFailure"},
	}
	options.SetOhSdkPluginExecutor(distributionFailureSDKPluginExecutor{})
	program := newOHSDKProgramWithOptions(fs, []string{
		"/sdk/@ohos.deviceInfo.d.ts", "/project/entry/src/main/ets/entry.ets",
	}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("retained distribution diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_validate_utils.ts::checkDistributionOSVersion passes minRequiredVersion
// (without the @Available OS prefix) to the since CompatibilityCheck callback.
func TestOHSDKApiAvailableGuardUsesPlainRequiredVersion(t *testing.T) {
	t.Parallel()
	compatible := float64(17)
	compile := float64(17)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.annotation.d.ets": `@interface Available { minApiVersion: string }`,
		"/sdk/@ohos.deviceInfo.d.ts":  `declare namespace deviceInfo { function apiAvailable(version: string | number): boolean; }`,
		"/project/library.ets": `
            @Available({ minApiVersion: "HarmonyOS 5.0.5(19)" })
            export function future(): void {}
        `,
		"/project/entry/src/main/ets/entry.ets": `
            if (deviceInfo.apiAvailable("5.0.5(20)")) {
              future();
            }
        `,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhRuntimeOS = "HarmonyOS"
	options.OhDeviceTypes = nil
	options.OhAllModulePaths = append(options.OhAllModulePaths, "/sdk/@ohos.annotation.d.ets")
	options.OhSdkCheckPlugins = []core.OhSdkCheckPlugin{
		{OSName: "HarmonyOS", Tag: "available", Type: "CompatibilityCheck", Path: "/sdk/available.js", FunctionName: "checkAvailableValue"},
		{OSName: "HarmonyOS", Tag: "available", Type: "FormatValidation", Path: "/sdk/format.js", FunctionName: "checkAvailableFormat"},
		{OSName: "HarmonyOS", Tag: "since", Type: "CompatibilityCheck", Path: "/sdk/since.js", FunctionName: "checkSinceValue"},
		{OSName: "HarmonyOS", Tag: "since", Path: "/sdk/distribution.js", FunctionName: "checkDistribution"},
	}
	options.SetOhSdkPluginExecutor(apiAvailableGuardSDKPluginExecutor{})
	program := newOHSDKProgramWithOptions(fs, []string{
		"/sdk/@ohos.annotation.d.ets", "/sdk/@ohos.deviceInfo.d.ts", "/project/library.ets", "/project/entry/src/main/ets/entry.ets",
	}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("apiAvailable guard diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_utils.ts::parseVersion represents M.S.F as M*10000+S*100+F
// for permission-tag range intersection.
func TestOHSDKPermissionVersionRangeUsesMSFValues(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `
            /**
             * @permission [since 10.1.0 - 10.2.0] ohos.permission.SKIPPED
             * @syscap SystemCapability.Base
             */
            declare function skipped(): void;
            /**
             * @permission [since 9.9.0 - 10.0.0] ohos.permission.REQUIRED
             * @syscap SystemCapability.Base
             */
            declare function required(): void;
        `,
		"/project/entry/src/main/ets/entry.ets": `skipped(); required();`,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "ohos.permission.REQUIRED") {
		t.Fatalf("permission-range diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// AnnotateSuppressWarningsValidator stops at the nearest node that has any
// SuppressWarnings decorator; an inner rule therefore shadows outer rules.
func TestOHSDKNearestSuppressWarningsDecoratorShadowsOuter(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `/**
             * @since 12
             * @syscap SystemCapability.Base
             */ declare function future(): void;`,
		"/project/entry/src/main/ets/entry.ets": `
            @SuppressWarnings({ rules: [SuppressWarningsType.COMPATIBILITY] })
            class Outer {
              @SuppressWarnings({ rules: [SuppressWarningsType.SYSCAP] })
              run(): void { future(); }
            }
        `,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "supported since SDK version 12") {
		t.Fatalf("nearest-suppressor diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_utils.ts::getJsDocNodeCheckConfig adds the tagless FindModule
// contract when a non-declaration .ts file consumes an ArkUI declaration.
func TestOHSDKArkUIUseFromTypeScriptReportsFindModule(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/arkui.d.ts":                       `/** @syscap SystemCapability.Base */ declare function Button(): void;`,
		"/project/entry/src/main/ets/helper.ts": `Button();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhAllModulePaths = []string{"/sdk/arkui.d.ts"}
	options.OhArkUIDeclarationDirs = []string{"/sdk"}
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/arkui.d.ts", "/project/entry/src/main/ets/helper.ts"}, options)
	diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	if len(diagnostics) != 1 || !strings.Contains(ohDiagnosticText(diagnostics[0]), "Cannot find name 'Button'") {
		t.Fatalf("ArkUI TypeScript diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// CommentSuppressWarningsValidator uses the closest comment position inside a
// fluent call chain, rather than the position of the containing statement.
func TestOHSDKCommentSuppressorFollowsFluentCallChain(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `
            /** @syscap SystemCapability.Base */
            interface Fluent {
              /** @syscap SystemCapability.Base */ base(): Fluent;
              /**
               * @since 12
               * @syscap SystemCapability.Base
               */ future(): Fluent;
            }
            /** @syscap SystemCapability.Base */ declare const fluent: Fluent;
        `,
		"/project/entry/src/main/ets/entry.ets": `
            fluent.base()
              // @SuppressWarnings compatibility
              .future();
        `,
	}, true))
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, ohSDKJSDocOptions(&compatible, &compile))
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("fluent-call suppressor diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

// api_check_permission.ts evaluates its queue exactly as written. In
// particular, a leading/trailing logical token does not invalidate a granted
// permission expression.
func TestOHSDKPermissionQueueMatchesSourceEvaluation(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(10)
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `
            /**
             * @permission and ohos.permission.CAMERA and
             * @syscap SystemCapability.Base
             */ declare function camera(): void;
        `,
		"/project/entry/src/main/ets/entry.ets": `camera();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhRequestPermissions = []string{"ohos.permission.CAMERA"}
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", "/project/entry/src/main/ets/entry.ets"}, options)
	if diagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil)); len(diagnostics) != 0 {
		t.Fatalf("permission-queue diagnostics = %v", diagnosticTexts(diagnostics))
	}
}

func TestOHSDKRequiredApplicationTags(t *testing.T) {
	t.Parallel()
	compatible := float64(10)
	compile := float64(12)
	entry := "/project/entry/src/main/ets/entry.ets"
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/sdk/@ohos.sample.d.ts": `declare namespace sample { function plain(): void; }`,
		entry:                    `sample.plain();`,
	}, true))
	options := ohSDKJSDocOptions(&compatible, &compile)
	options.OhCardEntryFiles = []string{entry}
	options.OhCrossplatform = core.TSTrue
	options.OhIgnoreCrossplatformCheck = core.TSTrue
	options.OhBundleType = "atomicService"
	program := newOHSDKProgramWithOptions(fs, []string{"/sdk/@ohos.sample.d.ts", entry}, options)
	if program.Options().OhBundleType != "atomicService" || program.Options().CompileSdkVersion == nil || *program.Options().CompileSdkVersion != 12 {
		t.Fatalf("SDK application options were not retained: %#v", program.Options())
	}
	sdkDiagnostics := ohSDKDiagnostics(program.GetSemanticDiagnostics(t.Context(), nil))
	for _, expected := range []struct {
		text     string
		category diagnostics.Category
	}{
		{"11706006#", diagnostics.CategoryError},
		{"11706007#", diagnostics.CategoryWarning},
		{"11706010#", diagnostics.CategoryError},
	} {
		if !slices.ContainsFunc(sdkDiagnostics, func(diagnostic *ast.Diagnostic) bool {
			return diagnostic.Category() == expected.category && strings.Contains(ohDiagnosticText(diagnostic), expected.text)
		}) {
			t.Fatalf("missing %#v in SDK diagnostics: %v", expected, diagnosticTexts(sdkDiagnostics))
		}
	}
}

func newOHSDKProgram(fs vfs.FS, files []string) *compiler.Program {
	options := &core.CompilerOptions{
		NoEmit:                 core.TSTrue,
		Module:                 core.ModuleKindESNext,
		ModuleResolution:       core.ModuleResolutionKindBundler,
		ExperimentalDecorators: core.TSTrue,
		EtsAnnotationsEnable:   core.TSTrue,
		Ets:                    etstest.Options(),
		OhRuntimeOS:            ohRuntimeOSForTest,
	}
	return compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       files,
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
}

func ohSDKJSDocOptions(compatible *float64, compile *float64) *core.CompilerOptions {
	return &core.CompilerOptions{
		NoEmit:                 core.TSTrue,
		Module:                 core.ModuleKindESNext,
		ModuleResolution:       core.ModuleResolutionKindBundler,
		ExperimentalDecorators: core.TSTrue,
		EtsAnnotationsEnable:   core.TSTrue,
		Ets:                    etstest.Options(),
		CompatibleSdkVersion:   compatible,
		CompileSdkVersion:      compile,
		OhRuntimeOS:            ohRuntimeOSForTest,
		OhProjectRootPath:      "/project",
		OhModulePath:           "/project/entry",
		OhAllModulePaths:       []string{"/sdk/@ohos.sample.d.ts", "/sdk/@ohos.deviceInfo.d.ts"},
		OhGlobalModulePaths:    []string{"/sdk"},
		OhDeviceTypes:          []string{"phone"},
		OhSyscapIntersection:   []string{"SystemCapability.Base"},
		OhSyscapUnion:          []string{"SystemCapability.Base", "SystemCapability.Camera"},
		OhCompileMode:          "moduleJson",
	}
}

func newOHSDKProgramWithOptions(fs vfs.FS, files []string, options *core.CompilerOptions) *compiler.Program {
	return compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       files,
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
	})
}

func ohSDKDiagnostics(all []*ast.Diagnostic) []*ast.Diagnostic {
	return core.Filter(all, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 28007 })
}

func diagnosticTexts(all []*ast.Diagnostic) []string {
	result := make([]string, 0, len(all))
	for _, diagnostic := range all {
		result = append(result, ohDiagnosticText(diagnostic))
	}
	return result
}

const ohRuntimeOSForTest = "OpenHarmony"

func ohDiagnosticText(diagnostic *ast.Diagnostic) string {
	return diagnosticwriter.WrapASTDiagnostic(diagnostic).Localize(locale.Default)
}

type harmonySDKPluginExecutor struct{}

type countingHarmonySDKPluginExecutor struct {
	harmonySDKPluginExecutor
	valueCalls atomic.Int32
}

type apiAvailableGuardSDKPluginExecutor struct{}

type syscapSDKPluginExecutor struct{ harmonySDKPluginExecutor }

type distributionFailureSDKPluginExecutor struct{ harmonySDKPluginExecutor }

func (e *countingHarmonySDKPluginExecutor) CheckValue(plugin core.OhSdkCheckPlugin, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool, error) {
	e.valueCalls.Add(1)
	return e.harmonySDKPluginExecutor.CheckValue(plugin, required, target, scene)
}

func (distributionFailureSDKPluginExecutor) CheckDistribution(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginDistributionResult, bool, error) {
	if plugin.FunctionName == "loadFailure" {
		return core.OhSdkPluginDistributionResult{}, false, errors.New("module load failed")
	}
	return harmonySDKPluginExecutor{}.CheckDistribution(plugin, version)
}

func (syscapSDKPluginExecutor) PrepareClass(core.OhSdkClassCheckPlugin) (bool, error) {
	return true, nil
}

func (syscapSDKPluginExecutor) CheckSyscap(plugin core.OhSdkClassCheckPlugin, request core.OhSdkClassCheckRequest) (core.OhSdkPluginSyscapResult, bool, error) {
	intersection, _ := request.ProjectConfig["syscapIntersectionSet"].([]string)
	strictMode, _ := request.ProjectConfig["strictMode"].(map[string]any)
	validRequest := plugin.ClassName == "SyscapChecker" && request.Node.Text == "current" &&
		strings.Contains(request.Declaration.Text, "declare function current") &&
		slices.Contains(intersection, "SystemCapability.Base") &&
		request.ProjectConfig["customPluginValue"] == "preserved" &&
		strictMode["apiCompatibilityCheck"] == "warning"
	return core.OhSdkPluginSyscapResult{
		CheckResult: validRequest, CheckMessage: "extension device excludes current",
	}, true, nil
}

func (apiAvailableGuardSDKPluginExecutor) CheckValue(plugin core.OhSdkCheckPlugin, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool, error) {
	switch plugin.FunctionName {
	case "checkAvailableValue":
		return core.OhSdkPluginCheckResult{Result: false}, true, nil
	case "checkSinceValue":
		return core.OhSdkPluginCheckResult{Result: required == "5.0.5(19)" && target == "5.0.5(20)" && scene == 1}, true, nil
	default:
		return core.OhSdkPluginCheckResult{}, false, nil
	}
}

func (apiAvailableGuardSDKPluginExecutor) PrepareClass(core.OhSdkClassCheckPlugin) (bool, error) {
	return false, nil
}

func (apiAvailableGuardSDKPluginExecutor) CheckFormat(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginCheckResult, bool, error) {
	return core.OhSdkPluginCheckResult{Result: plugin.FunctionName == "checkAvailableFormat" && version == "HarmonyOS 5.0.5(19)"}, true, nil
}

func (apiAvailableGuardSDKPluginExecutor) CheckDistribution(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginDistributionResult, bool, error) {
	if plugin.FunctionName != "checkDistribution" {
		return core.OhSdkPluginDistributionResult{}, false, nil
	}
	return core.OhSdkPluginDistributionResult{Valid: true, Version: version}, true, nil
}

func (apiAvailableGuardSDKPluginExecutor) MatchBuildVersion(core.OhSdkCheckPlugin, string) (core.OhSdkPluginRegexResult, bool, error) {
	return core.OhSdkPluginRegexResult{}, false, nil
}

func (apiAvailableGuardSDKPluginExecutor) CheckSyscap(core.OhSdkClassCheckPlugin, core.OhSdkClassCheckRequest) (core.OhSdkPluginSyscapResult, bool, error) {
	return core.OhSdkPluginSyscapResult{}, false, nil
}

func (harmonySDKPluginExecutor) CheckValue(plugin core.OhSdkCheckPlugin, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool, error) {
	return core.OhSdkPluginCheckResult{
		Result: plugin.FunctionName == "checkSinceValue" && required == "11" && target == "5.0.5(17)" && scene == 0,
	}, true, nil
}

func (harmonySDKPluginExecutor) PrepareClass(core.OhSdkClassCheckPlugin) (bool, error) {
	return false, nil
}

func (harmonySDKPluginExecutor) CheckFormat(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginCheckResult, bool, error) {
	if plugin.FunctionName != "checkAvailableFormat" {
		return core.OhSdkPluginCheckResult{}, false, nil
	}
	return core.OhSdkPluginCheckResult{Result: version == "HarmonyOS 5.0.5(17)"}, true, nil
}

func (harmonySDKPluginExecutor) CheckDistribution(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginDistributionResult, bool, error) {
	switch plugin.FunctionName {
	case "checkFirstDistribution":
		return core.OhSdkPluginDistributionResult{Valid: false, Version: "first"}, true, nil
	case "checkLastDistribution", "checkDistribution":
		return core.OhSdkPluginDistributionResult{Valid: version == "5.0.5(17)", Version: "17"}, true, nil
	default:
		return core.OhSdkPluginDistributionResult{}, false, nil
	}
}

func (harmonySDKPluginExecutor) MatchBuildVersion(core.OhSdkCheckPlugin, string) (core.OhSdkPluginRegexResult, bool, error) {
	return core.OhSdkPluginRegexResult{}, false, nil
}

func (harmonySDKPluginExecutor) CheckSyscap(core.OhSdkClassCheckPlugin, core.OhSdkClassCheckRequest) (core.OhSdkPluginSyscapResult, bool, error) {
	return core.OhSdkPluginSyscapResult{}, false, nil
}
