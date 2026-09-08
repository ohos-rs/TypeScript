package checker

import (
	"fmt"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/scanner"
)

const (
	ohSdkCompatibilityCheck = "CompatibilityCheck"
	ohSdkFormatValidation   = "FormatValidation"
)

func (c *Checker) ohSdkPlugins(tag string, pluginType string) []core.OhSdkCheckPlugin {
	result := make([]core.OhSdkCheckPlugin, 0)
	for _, plugin := range c.compilerOptions.OhSdkCheckPlugins {
		if plugin.OSName == c.ohRuntimeOS() && plugin.Tag == tag && plugin.Type == pluginType {
			result = append(result, plugin)
		}
	}
	return result
}

// api_check_utils.ts::initValueChecker prefers the typed key and falls back to
// the legacy {runtimeOS}/{tag} key only when no typed entry was registered.
func (c *Checker) ohSdkValuePlugins(tag string) []core.OhSdkCheckPlugin {
	plugins := c.ohSdkPlugins(tag, ohSdkCompatibilityCheck)
	if len(plugins) != 0 {
		return plugins
	}
	return c.ohSdkPlugins(tag, "")
}

func (c *Checker) ohSdkCheckValue(tag string, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool) {
	executor := c.compilerOptions.OhSdkPluginExecutor
	if executor == nil {
		return core.OhSdkPluginCheckResult{}, false
	}
	for _, plugin := range c.ohSdkValuePlugins(tag) {
		result, found, err := executor.CheckValue(plugin, required, target, scene)
		if err != nil {
			panic(fmt.Errorf("OH SDK value checker: %w", err))
		}
		if found {
			return result, true
		}
	}
	return core.OhSdkPluginCheckResult{}, false
}

func (c *Checker) ohSdkCheckFormat(tag string, version string) (core.OhSdkPluginCheckResult, bool) {
	executor := c.compilerOptions.OhSdkPluginExecutor
	if executor == nil {
		return core.OhSdkPluginCheckResult{}, false
	}
	for _, plugin := range c.ohSdkPlugins(tag, ohSdkFormatValidation) {
		result, found, err := executor.CheckFormat(plugin, version)
		if err != nil {
			panic(fmt.Errorf("OH SDK format checker: %w", err))
		}
		if found {
			return result, true
		}
	}
	return core.OhSdkPluginCheckResult{}, false
}

func (c *Checker) ohSdkCheckDistribution(tag string, version string) (core.OhSdkPluginDistributionResult, bool) {
	executor := c.compilerOptions.OhSdkPluginExecutor
	if executor == nil {
		return core.OhSdkPluginDistributionResult{}, false
	}
	for _, plugin := range c.ohSdkPlugins(tag, "") {
		result, found, err := executor.CheckDistribution(plugin, version)
		if err != nil {
			// isCheckDistributionOSVersion catches callback errors and returns its
			// initial invalid result without trying another registered function.
			return core.OhSdkPluginDistributionResult{}, false
		}
		if found {
			return result, true
		}
	}
	return core.OhSdkPluginDistributionResult{}, false
}

func (c *Checker) ohSdkMatchBuildVersion(tag string, version string) (core.OhSdkPluginRegexResult, bool) {
	executor := c.compilerOptions.OhSdkPluginExecutor
	if executor == nil {
		return core.OhSdkPluginRegexResult{}, false
	}
	for _, plugin := range c.ohSdkPlugins(tag, "getBuildVersionRegex") {
		result, found, err := executor.MatchBuildVersion(plugin, version)
		if err != nil {
			// api_check_utils.ts::getBuildVersionRegex catches both module loading
			// and callback invocation errors and returns undefined.
			return core.OhSdkPluginRegexResult{}, false
		}
		if found {
			return result, true
		}
	}
	return core.OhSdkPluginRegexResult{}, false
}

func (c *Checker) ohSdkCheckSyscap(node *ast.Node, declaration *ast.Node) (core.OhSdkPluginSyscapResult, bool) {
	executor := c.compilerOptions.OhSdkPluginExecutor
	if executor == nil {
		return core.OhSdkPluginSyscapResult{}, false
	}
	request := core.OhSdkClassCheckRequest{
		Node:          ohSdkPluginNodeSnapshot(node),
		Declaration:   ohSdkPluginNodeSnapshot(declaration),
		ProjectConfig: c.ohSdkPluginProjectConfig(),
	}
	configured := false
	result := core.OhSdkPluginSyscapResult{}
	for _, plugin := range c.compilerOptions.OhSdkClassCheckPlugins {
		if plugin.TagName != "syscap" {
			continue
		}
		current, found, err := executor.CheckSyscap(plugin, request)
		if err != nil {
			panic(fmt.Errorf("OH SDK syscap checker: %w", err))
		}
		if !found {
			// collectExternalApiChecker omits modules/classes that cannot be
			// loaded, so they are not entries in externalApiCheckerMap.
			continue
		}
		configured = true
		result = current
		// checkSyscapAbility returns immediately for a checker without check()
		// or for the first callback that reports checkResult=false.
		if !current.CheckResult {
			return current, true
		}
	}
	return result, configured
}

func ohSdkPluginNodeSnapshot(node *ast.Node) core.OhSdkPluginNodeSnapshot {
	file := ast.GetSourceFileOfNode(node)
	return core.OhSdkPluginNodeSnapshot{
		FileName: file.FileName(),
		Source:   file.Text(),
		Pos:      node.Pos(),
		End:      node.End(),
		Text:     scanner.GetTextOfNode(node),
	}
}

func (c *Checker) ohSdkPluginProjectConfig() core.OhSdkPluginProjectConfig {
	return core.OhSdkPluginProjectConfig{
		RuntimeOS:                  c.ohRuntimeOS(),
		OriginCompatibleSdkVersion: c.compilerOptions.OhOriginCompatibleSdkVersion,
		CompatibleSdkVersion:       c.compilerOptions.CompatibleSdkVersion,
		CompileSdkVersion:          c.compilerOptions.CompileSdkVersion,
		ProjectRootPath:            c.compilerOptions.OhProjectRootPath,
		ProjectPath:                c.compilerOptions.OhProjectPath,
		ModulePath:                 c.compilerOptions.OhModulePath,
		EtsLoaderPath:              c.compilerOptions.EtsLoaderPath,
		ExternalApiPaths:           c.compilerOptions.OhExternalApiPaths,
		RequestPermissions:         c.compilerOptions.OhRequestPermissions,
		SyscapIntersection:         c.compilerOptions.OhSyscapIntersection,
		SyscapUnion:                c.compilerOptions.OhSyscapUnion,
		DeviceTypes:                c.compilerOptions.OhDeviceTypes,
		Crossplatform:              c.compilerOptions.OhCrossplatform,
		IgnoreCrossplatformCheck:   c.compilerOptions.OhIgnoreCrossplatformCheck,
		CompileMode:                c.compilerOptions.OhCompileMode,
		BundleType:                 c.compilerOptions.OhBundleType,
		ApiCompatibilityCheck:      c.compilerOptions.OhApiCompatibilityCheck,
	}
}
