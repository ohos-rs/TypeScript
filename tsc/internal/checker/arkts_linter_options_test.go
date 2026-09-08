package checker

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

// ArkTSLinter_1_1/Utils.ts::configureStrictCheckOHModule owns the exact
// directory-component policy used by isLibrarySymbol. An explicitly empty
// disableStrictCheckPaths differs from an absent value, and enabling strict
// checking for oh_modules removes only that one default directory.
func TestArkTSLinterConfiguredThirdPartyDirectories(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fileName string
		paths    []string
		enableOH core.Tristate
		want     bool
	}{
		{name: "default node_modules", fileName: "/project/node_modules/lib.ets", want: true},
		{name: "default oh_modules", fileName: "/project/oh_modules/lib.ets", want: true},
		{name: "explicit empty", fileName: "/project/node_modules/lib.ets", paths: []string{}, want: false},
		{name: "custom directory", fileName: "/project/vendor/lib.ets", paths: []string{"", "vendor", "vendor"}, want: true},
		{name: "enable strict oh_modules", fileName: "/project/oh_modules/lib.ets", enableOH: core.TSTrue, want: false},
		{name: "keep node_modules when enabling oh_modules", fileName: "/project/node_modules/lib.ets", enableOH: core.TSTrue, want: true},
		{name: "case sensitive component", fileName: "/project/Node_Modules/lib.ets", want: false},
		{name: "ignored file", fileName: "/project/hvigorfile.ts", paths: []string{}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{
				FileName: test.fileName,
				Path:     tspath.Path(test.fileName),
			}, "class LibraryType {}", core.ScriptKindETS)
			checker := &Checker{compilerOptions: &core.CompilerOptions{
				DisableStrictCheckPaths:   test.paths,
				EnableStrictCheckOHModule: test.enableOH,
			}}
			linter := arkTSLinter{
				checker:            checker,
				ignoredDirectories: checker.arkTSIgnoredDirectories(),
				pathComponents:     make(map[*ast.SourceFile][]string),
			}
			if got := linter.isThirdPartyFile(file); got != test.want {
				t.Fatalf("isThirdPartyFile(%q) = %v, want %v", test.fileName, got, test.want)
			}
		})
	}
}
