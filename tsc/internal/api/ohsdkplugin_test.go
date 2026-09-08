package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/json"
)

type sdkPluginCallbackConn struct {
	requests []ohSdkPluginRequest
}

func (*sdkPluginCallbackConn) Run(context.Context) error { return nil }

func (c *sdkPluginCallbackConn) Call(_ context.Context, method string, params any) (json.Value, error) {
	if method != ohSdkPluginCallback {
		return nil, fmt.Errorf("unexpected callback %q", method)
	}
	request, ok := params.(ohSdkPluginRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected request type %T", params)
	}
	c.requests = append(c.requests, request)
	var response ohSdkPluginResponse
	switch request.Operation {
	case "prepareClass":
		response.Found = request.ClassName == "SyscapChecker"
	case "value":
		response = ohSdkPluginResponse{Found: true, Result: true, Message: "value"}
	case "format":
		response = ohSdkPluginResponse{Found: true, Result: true, Message: "format"}
	case "distribution":
		response = ohSdkPluginResponse{Found: true, Valid: true, Version: "17", Message: "distribution"}
	case "regex":
		response = ohSdkPluginResponse{Found: true, Matched: true, Groups: []string{"5.0.5(17)", "5", "0", "5", "17"}}
	case "syscap":
		response = ohSdkPluginResponse{Found: true, CheckResult: true, CheckMessage: "syscap"}
	default:
		response = ohSdkPluginResponse{Found: false}
	}
	return json.Marshal(response)
}

func (*sdkPluginCallbackConn) Notify(context.Context, string, any) error { return nil }

func TestClientOhSdkPluginExecutorPreservesTypedCallbackContracts(t *testing.T) {
	conn := &sdkPluginCallbackConn{}
	executor := newClientOhSdkPluginExecutor()
	executor.SetConnection(context.Background(), conn)
	plugin := core.OhSdkCheckPlugin{Path: "/sdk/checker.cjs"}

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
	if err != nil || !found || !distribution.Valid || distribution.Version != "17" {
		t.Fatalf("distribution result = %#v, found %v, error %v", distribution, found, err)
	}
	plugin.FunctionName = "regex"
	regex, found, err := executor.MatchBuildVersion(plugin, "5.0.5(17)")
	if err != nil || !found || !regex.Matched || len(regex.Groups) != 5 || regex.Groups[4] != "17" {
		t.Fatalf("regex result = %#v, found %v, error %v", regex, found, err)
	}
	classPlugin := core.OhSdkClassCheckPlugin{
		TagName: "syscap", Path: plugin.Path, ClassName: "SyscapChecker",
	}
	if found, err := executor.PrepareClass(classPlugin); err != nil || !found {
		t.Fatalf("prepare class found %v, error %v", found, err)
	}
	syscap, found, err := executor.CheckSyscap(classPlugin, core.OhSdkClassCheckRequest{})
	if err != nil || !found || !syscap.CheckResult || syscap.CheckMessage != "syscap" {
		t.Fatalf("syscap result = %#v, found %v, error %v", syscap, found, err)
	}

	if len(conn.requests) != 6 {
		t.Fatalf("received %d callbacks, want 6", len(conn.requests))
	}
	if conn.requests[0].Path != plugin.Path || conn.requests[0].Operation != "value" {
		t.Fatalf("first callback = %#v", conn.requests[0])
	}
	if conn.requests[4].ClassName != classPlugin.ClassName {
		t.Fatalf("class callback = %#v", conn.requests[4])
	}
}

func TestClientOhSdkPluginExecutorPreservesLoadFailurePolicy(t *testing.T) {
	executor := newClientOhSdkPluginExecutor()
	executor.SetConnection(context.Background(), sdkPluginErrorConn{})
	plugin := core.OhSdkCheckPlugin{Path: "/missing.cjs", FunctionName: "check"}
	if _, found, err := executor.CheckValue(plugin, "1", "1", 0); err != nil || found {
		t.Fatalf("value load failure found %v, error %v", found, err)
	}
	if _, found, err := executor.CheckDistribution(plugin, "1"); err == nil || found {
		t.Fatalf("distribution load failure found %v, error %v", found, err)
	}
}

type sdkPluginErrorConn struct{}

func (sdkPluginErrorConn) Run(context.Context) error                 { return nil }
func (sdkPluginErrorConn) Notify(context.Context, string, any) error { return nil }
func (sdkPluginErrorConn) Call(context.Context, string, any) (json.Value, error) {
	return json.Marshal(ohSdkPluginResponse{Phase: "load", Error: "module not found"})
}
