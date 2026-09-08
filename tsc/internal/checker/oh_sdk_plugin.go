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
	executor := c.compilerOptions.GetOhSdkPluginExecutor()
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
	executor := c.compilerOptions.GetOhSdkPluginExecutor()
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
	executor := c.compilerOptions.GetOhSdkPluginExecutor()
	if executor == nil {
		return core.OhSdkPluginDistributionResult{}, false
	}
	result := core.OhSdkPluginDistributionResult{}
	configured := false
	for _, plugin := range c.ohSdkPlugins(tag, "") {
		current, found, err := executor.CheckDistribution(plugin, version)
		if err != nil {
			// isCheckDistributionOSVersion returns the value retained from the
			// preceding callback when a later load/invocation throws.
			return result, configured
		}
		if found {
			configured = true
			result = current
		}
	}
	return result, configured
}

func (c *Checker) ohSdkMatchBuildVersion(tag string, version string) (core.OhSdkPluginRegexResult, bool) {
	executor := c.compilerOptions.GetOhSdkPluginExecutor()
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
	executor := c.compilerOptions.GetOhSdkPluginExecutor()
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
	positionMap := file.GetPositionMap()
	return core.OhSdkPluginNodeSnapshot{
		FileName: file.FileName(),
		Source:   file.Text(),
		Pos:      positionMap.UTF8ToUTF16(node.Pos()),
		End:      positionMap.UTF8ToUTF16(node.End()),
		Text:     scanner.GetTextOfNode(node),
	}
}

func (c *Checker) ohSdkPluginProjectConfig() map[string]any {
	if source := c.compilerOptions.OhSdkPluginProjectConfig; source != nil {
		result := make(map[string]any, len(source))
		for key, value := range source {
			result[key] = value
		}
		return result
	}
	return map[string]any{
		"runtimeOS":                  c.ohRuntimeOS(),
		"originCompatibleSdkVersion": c.compilerOptions.OhOriginCompatibleSdkVersion,
		"compatibleSdkVersion":       c.compilerOptions.CompatibleSdkVersion,
		"compileSdkVersion":          c.compilerOptions.CompileSdkVersion,
		"projectRootPath":            c.compilerOptions.OhProjectRootPath,
		"projectPath":                c.compilerOptions.OhProjectPath,
		"modulePath":                 c.compilerOptions.OhModulePath,
		"etsLoaderPath":              c.compilerOptions.EtsLoaderPath,
		"externalApiPaths":           c.compilerOptions.OhExternalApiPaths,
		"requestPermissions":         c.compilerOptions.OhRequestPermissions,
		"syscapIntersectionSet":      c.compilerOptions.OhSyscapIntersection,
		"syscapUnionSet":             c.compilerOptions.OhSyscapUnion,
		"deviceTypes":                c.compilerOptions.OhDeviceTypes,
		"isCrossplatform":            c.compilerOptions.OhCrossplatform == core.TSTrue,
		"ignoreCrossplatformCheck":   c.compilerOptions.OhIgnoreCrossplatformCheck == core.TSTrue,
		"compileMode":                c.compilerOptions.OhCompileMode,
		"bundleType":                 c.compilerOptions.OhBundleType,
		"strictMode": map[string]any{
			"apiCompatibilityCheck": c.compilerOptions.OhApiCompatibilityCheck,
		},
	}
}
