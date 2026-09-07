package compiler_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/bundled"
	"github.com/microsoft/TypeScript/tsc/internal/compiler"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnosticwriter"
	"github.com/microsoft/TypeScript/tsc/internal/locale"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
	"github.com/microsoft/TypeScript/tsc/internal/tsoptions"
	"github.com/microsoft/TypeScript/tsc/internal/vfs"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/osvfs"
	"github.com/microsoft/TypeScript/tsc/internal/vfs/vfstest"
)

func TestArkTSLinterIsIndependentAndUsesStrictChecker(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/input.ets": "var value: string | undefined;\nconst copy: string = value;\n",
	}, true))
	program := newArkTSLinterProgram(fs, "/", []string{"/input.ets"})

	semantic := program.GetSemanticDiagnostics(t.Context(), nil)
	if slices.ContainsFunc(semantic, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 5 }) {
		t.Fatalf("ordinary semantic diagnostics unexpectedly contain ArkTS linter rule: %v", semantic)
	}
	linter := program.GetArkTSLinterDiagnostics(t.Context(), nil)
	if !slices.ContainsFunc(linter, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 5 }) {
		t.Fatalf("missing arkts-no-var diagnostic: %v", linter)
	}
	if !slices.ContainsFunc(linter, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == 2322 }) {
		t.Fatalf("missing strict-only assignment diagnostic: %v", linter)
	}
}

// OH program.ts::getBindAndCheckDiagnosticsForFileNoCache and
// LibraryTypeCallDiagnosticChecker.ts::rebuildTscDiagnostics make the linter
// checker the sole ETS semantic owner in strictCheckerOnly mode. Diagnostics
// outside the linter's filterable set are merged back after filtering.
func TestArkTSStrictCheckerOnlyOwnsETSSemanticDiagnostics(t *testing.T) {
	t.Parallel()
	fs := bundled.WrapFS(vfstest.FromMap(map[string]string{
		"/input.ets": "const value: string = undefined;\nfunction missing(): number {}\n",
	}, true))
	program := newArkTSLinterProgram(fs, "/", []string{"/input.ets"})
	program.Options().StrictCheckerOnly = core.TSTrue
	file := program.GetSourceFile("/input.ets")
	if diagnostics := program.GetSemanticDiagnostics(t.Context(), file); len(diagnostics) != 0 {
		t.Fatalf("ordinary ETS semantic diagnostics = %v, want none", diagnostics)
	}
	linter := program.GetArkTSLinterDiagnostics(t.Context(), file)
	for _, code := range []int32{2322, 2355} {
		if !slices.ContainsFunc(linter, func(diagnostic *ast.Diagnostic) bool { return diagnostic.Code() == code }) {
			t.Fatalf("strictCheckerOnly diagnostics missing TS%d: %v", code, linter)
		}
	}
}

// InteropTypescriptLinter.ts is a separate source-owned ruleset for TS files.
// LinterRunner.ts enables it only for tsImportSendableEnable (or SDK sources)
// and does not publish strict-checker diagnostics for those files.
func TestArkTSInteropLinter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		entry    string
		files    map[string]string
		expected []int32
	}{
		{
			name:  "ETS import forms",
			entry: "/entry.ts",
			files: map[string]string{
				"/entry.ts":   "import { Good, Bad } from '/target.ets';\nimport * as all from '/target.ets';\nimport '/target.ets';\nGood; Bad; all;\n",
				"/target.ets": "@Sendable\nexport class Good {}\nexport class Bad {}\n",
			},
			expected: []int32{165, 169, 170},
		},
		{
			name:  "re-export ETS",
			entry: "/entry.ts",
			files: map[string]string{
				"/entry.ts":   "export { Good } from '/target.ets';\n",
				"/target.ets": "@Sendable\nexport class Good {}\n",
			},
			expected: []int32{168},
		},
		{
			name:  "Sendable use",
			entry: "/entry.ts",
			files: map[string]string{
				"/entry.ts":   "import { Good, Box } from '/target.ets';\nclass Derived extends Good {}\nnew Box<object>();\nconst value: Good = {};\n1 as Good;\n",
				"/target.ets": "@Sendable\nexport class Good {}\n@Sendable\nexport class Box<T> {}\n",
			},
			expected: []int32{166, 156, 159, 161},
		},
		{
			name:  "SDK exports",
			entry: "/sdk/ets/api/entry.ts",
			files: map[string]string{
				"/sdk/ets/api/entry.ts": "@Sendable\nclass Local {}\nexport { Local };\nexport default Local;\n",
			},
			expected: []int32{167, 167},
		},
		{
			name:  "Kit symbol source",
			entry: "/entry.ts",
			files: map[string]string{
				"/entry.ts":                   "import { Plain } from '/sdk/ets/api/@kit.Test';\nPlain;\n",
				"/sdk/ets/api/@kit.Test.d.ts": "export class Plain {}\n",
				"/sdk/ets/build-tools/ets-loader/kit_configs/@kit.Test.json": `{"symbols":{"Plain":{"source":"plain.d.ets","bindings":"Plain"}}}`,
			},
			expected: []int32{165},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := bundled.WrapFS(vfstest.FromMap(tc.files, true))
			options := &core.CompilerOptions{
				NoEmit:                     core.TSTrue,
				Module:                     core.ModuleKindES2020,
				ModuleResolution:           core.ModuleResolutionKindNode10,
				Target:                     core.ScriptTargetES2021,
				AllowImportingTsExtensions: core.TSTrue,
				ExperimentalDecorators:     core.TSTrue,
				EtsAnnotationsEnable:       core.TSTrue,
				NeedDoArkTsLinter:          core.TSTrue,
				TsImportSendableEnable:     core.TSTrue,
				EtsLoaderPath:              "/sdk/ets/build-tools/ets-loader",
				Ets:                        etstest.Options(),
			}
			program := compiler.NewProgram(compiler.ProgramOptions{
				Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
					FileNames:       []string{tc.entry},
					CompilerOptions: options,
				}},
				Host: compiler.NewCompilerHost("/", fs, bundled.LibPath(), nil, nil, nil),
			})
			entry := program.GetSourceFile(tc.entry)
			if entry == nil {
				t.Fatal("entry source file was not loaded")
			}
			diagnostics := program.GetArkTSLinterDiagnostics(t.Context(), entry)
			actual := make([]int32, 0, len(diagnostics))
			for _, diagnostic := range diagnostics {
				actual = append(actual, diagnostic.Code())
			}
			if !slices.Equal(actual, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

// TestArkTSLinterOpenHarmonyCorpus is the source-parity harness for
// third_party_typescript/tests/arkTSTest/testcase. It is opt-in because the
// upstream checkout is intentionally not vendored. Set OH_TYPESCRIPT_SOURCE to
// that checkout; ARKTS_LINTER_CORPUS_FILTER may select one rule directory or
// file substring while iterating.
func TestArkTSLinterOpenHarmonyCorpus(t *testing.T) {
	root := os.Getenv("OH_TYPESCRIPT_SOURCE")
	if root == "" {
		t.Skip("OH_TYPESCRIPT_SOURCE is not set")
	}
	testcaseRoot := filepath.Join(root, "tests", "arkTSTest", "testcase")
	filter := os.Getenv("ARKTS_LINTER_CORPUS_FILTER")
	var expectationFiles []string
	err := filepath.WalkDir(testcaseRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") && (filter == "" || strings.Contains(path, filter)) {
			expectationFiles = append(expectationFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(expectationFiles)
	for _, expectationFile := range expectationFiles {
		expectationFile := expectationFile
		relative, _ := filepath.Rel(testcaseRoot, expectationFile)
		t.Run(relative, func(t *testing.T) {
			expected := readArkTS11Expectation(t, expectationFile)
			if expected == nil {
				t.Skip("not an ArkTS linter expectation")
			}
			sourceFile := strings.TrimSuffix(expectationFile, ".json") + ".ets"
			if _, err := os.Stat(sourceFile); err != nil {
				sourceFile = strings.TrimSuffix(expectationFile, ".json") + ".ts"
			}
			if _, err := os.Stat(sourceFile); err != nil {
				t.Skip("expectation has no matching .ets or .ts source")
			}

			files := []string{sourceFile}
			entries, err := os.ReadDir(filepath.Dir(sourceFile))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() || filepath.Join(filepath.Dir(sourceFile), name) == sourceFile {
					continue
				}
				if strings.HasSuffix(name, ".d.ts") || strings.HasSuffix(name, ".d.ets") {
					files = append(files, filepath.Join(filepath.Dir(sourceFile), name))
				}
			}
			program := newArkTSLinterProgram(bundled.WrapFS(osvfs.FS()), filepath.Dir(sourceFile), files)
			diagnostics := program.GetArkTSLinterDiagnostics(t.Context(), nil)
			actual := make([]arkTSLinterExpectation, 0, len(diagnostics))
			for _, diagnostic := range diagnostics {
				if diagnostic.File() == nil {
					continue
				}
				line, character := scanner.GetECMALineAndUTF16CharacterOfPosition(diagnostic.File(), diagnostic.Pos())
				actual = append(actual, arkTSLinterExpectation{
					MessageText: diagnosticwriter.WrapASTDiagnostic(diagnostic).Localize(locale.Default),
					Position:    arkTSLinterPosition{Line: line + 1, Character: int(character) + 1},
				})
			}
			if !slices.Equal(expected, actual) {
				t.Fatalf("ArkTS 1.1 linter mismatch\nexpected: %#v\nactual:   %#v", expected, actual)
			}
		})
	}
}

type arkTSLinterExpectationFile struct {
	ArkTS11 []arkTSLinterExpectation `json:"arktsVersion_1_1"`
}

type arkTSLinterExpectation struct {
	MessageText string              `json:"messageText"`
	Position    arkTSLinterPosition `json:"expectLineAndCharacter"`
}

type arkTSLinterPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func readArkTS11Expectation(t *testing.T, fileName string) []arkTSLinterExpectation {
	t.Helper()
	content, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatal(err)
	}
	var result arkTSLinterExpectationFile
	if err := json.Unmarshal(content, &result); err != nil {
		return nil
	}
	return result.ArkTS11
}

func newArkTSLinterProgram(fs vfs.FS, currentDirectory string, files []string) *compiler.Program {
	compatibleStage := "beta3"
	if len(files) > 0 && strings.Contains(files[0], "arkts-sendable-beta-compatible") {
		// tests/arkTSTest/run.js uses beta3 for the corpus and reruns this
		// directory with --enableBeta2 (see ignorecase.json).
		compatibleStage = "beta2"
	}
	compatibleVersion := float64(12)
	options := &core.CompilerOptions{
		NoEmit:                    core.TSTrue,
		Strict:                    core.TSFalse,
		Module:                    core.ModuleKindES2020,
		ModuleResolution:          core.ModuleResolutionKindNode10,
		Target:                    core.ScriptTargetES2021,
		Lib:                       []string{"lib.es2021.d.ts"},
		SkipLibCheck:              core.TSTrue,
		ExperimentalDecorators:    core.TSTrue,
		EtsAnnotationsEnable:      core.TSTrue,
		NeedDoArkTsLinter:         core.TSTrue,
		CompatibleSdkVersion:      &compatibleVersion,
		CompatibleSdkVersionStage: compatibleStage,
		Ets:                       etstest.Options(),
	}
	return compiler.NewProgram(compiler.ProgramOptions{
		Config: &tsoptions.ParsedCommandLine{ParsedConfig: &tsoptions.ParsedOptions{
			FileNames:       files,
			CompilerOptions: options,
		}},
		Host: compiler.NewCompilerHost(currentDirectory, fs, bundled.LibPath(), nil, nil, nil),
	})
}
