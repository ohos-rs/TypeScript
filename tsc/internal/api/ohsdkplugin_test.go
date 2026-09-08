package api

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
)

func TestNodeOhSdkPluginExecutorPreservesFunctionContracts(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is not installed")
	}
	pluginPath := filepath.Join(t.TempDir(), "sdk-checker.cjs")
	if err := os.WriteFile(pluginPath, []byte(`
exports.value = (required, target, scene) => {
  console.log('plugin output must not corrupt the protocol');
  return { result: required === '11' && target === '5.0.5(17)' && scene === 0, message: 'value' };
};
exports.format = version => ({ result: version === 'HarmonyOS 5.0.5(17)', message: 'format' });
exports.distribution = version => ({ valid: true, version: '17', message: version });
exports.regex = () => /^(\d+)\.(\d+)\.(\d+)\((\d+)\)$/;
let syscapCheckerConstructions = 0;
exports.SyscapChecker = class {
  constructor() { syscapCheckerConstructions++; }
  check(node, declaration, projectConfig) {
    return {
      checkResult: syscapCheckerConstructions === 1 && node.getText() === 'current' &&
        declaration.getText().includes('declare function current') &&
        projectConfig.runtimeOS === 'HarmonyOS' &&
        projectConfig.syscapIntersectionSet.has('SystemCapability.Base') &&
        projectConfig.syscapUnionSet.has('SystemCapability.Extension'),
      checkMessage: 'syscap'
    };
  }
};
`), 0o600); err != nil {
		t.Fatal(err)
	}
	executor := newNodeOhSdkPluginExecutor(context.Background(), node, io.Discard)
	t.Cleanup(func() { _ = executor.Close() })
	plugin := core.OhSdkCheckPlugin{Path: pluginPath}

	plugin.FunctionName = "value"
	value, found, err := executor.CheckValue(plugin, "11", "5.0.5(17)", 0)
	if err != nil || !found || !value.Result || value.Message != "value" {
		t.Fatalf("value result = %#v, found %v, error %v", value, found, err)
	}
	plugin.FunctionName = "format"
	format, found, err := executor.CheckFormat(plugin, "HarmonyOS 5.0.5(17)")
	if err != nil || !found || !format.Result || format.Message != "format" {
		t.Fatalf("format result = %#v, found %v, error %v", format, found, err)
	}
	plugin.FunctionName = "distribution"
	distribution, found, err := executor.CheckDistribution(plugin, "5.0.5(17)")
	if err != nil || !found || !distribution.Valid || distribution.Version != "17" || distribution.Message != "5.0.5(17)" {
		t.Fatalf("distribution result = %#v, found %v, error %v", distribution, found, err)
	}
	plugin.FunctionName = "regex"
	regex, found, err := executor.MatchBuildVersion(plugin, "5.0.5(17)")
	if err != nil || !found || !regex.Matched || len(regex.Groups) != 5 || regex.Groups[4] != "17" {
		t.Fatalf("regex result = %#v, found %v, error %v", regex, found, err)
	}
	plugin.FunctionName = "missing"
	_, found, err = executor.CheckFormat(plugin, "ignored")
	if err != nil || found {
		t.Fatalf("missing export found %v, error %v", found, err)
	}
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	projectSource := "current();"
	declarationSource := "declare function current(): void;"
	classPlugin := core.OhSdkClassCheckPlugin{
		TagName: "syscap", Path: pluginPath, ClassName: "SyscapChecker",
	}
	if found, err := executor.PrepareClass(classPlugin); err != nil || !found {
		t.Fatalf("prepare class found %v, error %v", found, err)
	}
	syscap, found, err := executor.CheckSyscap(classPlugin, core.OhSdkClassCheckRequest{
		Node: core.OhSdkPluginNodeSnapshot{
			FileName: "/project/input.ts", Source: projectSource,
			Pos: 0, End: len("current"), Text: "current",
		},
		Declaration: core.OhSdkPluginNodeSnapshot{
			FileName: "/sdk/api.d.ts", Source: declarationSource,
			Pos: 0, End: len(declarationSource), Text: declarationSource,
		},
		ProjectConfig: core.OhSdkPluginProjectConfig{
			RuntimeOS: "HarmonyOS", EtsLoaderPath: repoRoot,
			SyscapIntersection: []string{"SystemCapability.Base"},
			SyscapUnion:        []string{"SystemCapability.Extension"},
		},
	})
	if err != nil || !found || !syscap.CheckResult || syscap.CheckMessage != "syscap" {
		t.Fatalf("syscap result = %#v, found %v, error %v", syscap, found, err)
	}
}
