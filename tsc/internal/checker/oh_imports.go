package checker

import (
	"strings"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/diagnostics"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

func (c *Checker) reportUncheckedSoModule(node *ast.Node, name string) {
	c.error(node, diagnostics.Currently_module_for_0_is_not_verified_If_you_re_importing_napi_its_verification_will_be_enabled_in_later_SDK_version_Please_make_sure_the_corresponding_d_ts_file_is_provided_and_the_napis_are_correctly_declared, name)
}

// OH checker.ts::resolveExternalModule/allowImportSendable. The source tests
// ScriptKind.TS, not all non-ETS files, and SDK membership is a string prefix
// (not a filesystem containment check). Dynamic/import-type/require syntax is
// never exempted by the Sendable switch. This supplies checking only, no emit.
func (c *Checker) checkTsImportEts(source *ast.SourceFile, specifier, errorNode *ast.Node) {
	if errorNode == nil || source.ScriptKind != core.ScriptKindTS || c.compilerOptions.NeedDoArkTsLinter != core.TSTrue {
		return
	}
	allowed := c.compilerOptions.TsImportSendableEnable == core.TSTrue && !source.IsDeclarationFile
	if loader := c.compilerOptions.EtsLoaderPath; loader != "" && source.IsDeclarationFile {
		sdkPath := tspath.GetNormalizedAbsolutePath("../..", loader)
		allowed = strings.HasPrefix(tspath.NormalizePath(source.FileName()), sdkPath)
	}
	if specifier != nil && !ast.IsImportDeclaration(specifier.Parent) && !ast.IsExportDeclaration(specifier.Parent) || !allowed {
		message := diagnostics.Importing_ArkTS_files_in_JS_and_TS_files_is_forbidden
		if c.compilerOptions.IsCompatibleVersion == core.TSTrue {
			message = diagnostics.Importing_ArkTS_files_in_JS_and_TS_files_is_about_to_be_forbidden
		}
		c.error(errorNode, message)
	}
}
