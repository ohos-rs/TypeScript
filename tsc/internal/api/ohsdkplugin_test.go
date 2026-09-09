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
	requests, ok := params.([]ohSdkPluginRequest)
	if !ok {
		return nil, fmt.Errorf("unexpected request type %T", params)
	}
	c.requests = append(c.requests, requests...)
	responses := make([]ohSdkPluginResponse, len(requests))
	for index, request := range requests {
		switch request.Operation {
		case "prepareClass":
			responses[index].Found = request.ClassName == "SyscapChecker"
		case "value":
			responses[index] = ohSdkPluginResponse{Found: true, Result: true, Message: "value"}
		case "format":
			responses[index] = ohSdkPluginResponse{Found: true, Result: true, Message: "format"}
		case "distribution":
			responses[index] = ohSdkPluginResponse{Found: true, Valid: true, Version: "17", Message: "distribution"}
		case "regex":
			responses[index] = ohSdkPluginResponse{Found: true, Matched: true, Groups: []string{"5.0.5(17)", "5", "0", "5", "17"}}
		case "syscap":
			responses[index] = ohSdkPluginResponse{Found: true, CheckResult: true, CheckMessage: "syscap"}
		}
	}
	return json.Marshal(responses)
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
	classRequest := core.OhSdkClassCheckRequest{
		Node: core.OhSdkPluginNodeSnapshot{
			FileName: "/project/Index.ets", Source: "use API", Pos: 0, End: 3, Text: "use",
		},
		Declaration: core.OhSdkPluginNodeSnapshot{
			FileName: "/sdk/api.d.ets", Source: "declare API", Pos: 8, End: 11, Text: "API",
		},
		ProjectConfig: map[string]any{"etsLoaderPath": "/sdk/loader"},
	}
	syscap, found, err := executor.CheckSyscap(classPlugin, classRequest)
	if err != nil || !found || !syscap.CheckResult || syscap.CheckMessage != "syscap" {
		t.Fatalf("syscap result = %#v, found %v, error %v", syscap, found, err)
	}
	if _, _, err := executor.CheckSyscap(classPlugin, classRequest); err != nil {
		t.Fatalf("second syscap callback failed: %v", err)
	}
	changedClassRequest := classRequest
	changedClassRequest.Node.Pos = 1
	changedClassRequest.Node.Text = "se"
	if _, _, err := executor.CheckSyscap(classPlugin, changedClassRequest); err != nil {
		t.Fatalf("changed syscap callback failed: %v", err)
	}

	if len(conn.requests) != 8 {
		t.Fatalf("received %d callbacks, want 8", len(conn.requests))
	}
	if conn.requests[0].SessionID == 0 || conn.requests[0].Path != plugin.Path || conn.requests[0].Operation != "value" {
		t.Fatalf("first callback = %#v", conn.requests[0])
	}
	if conn.requests[4].ClassName != classPlugin.ClassName {
		t.Fatalf("class callback = %#v", conn.requests[4])
	}
	first, ok := conn.requests[5].Args[0].(clientOhSdkClassCheckRequest)
	if !ok || first.Node.Source == nil || first.Declaration.Source == nil || first.ProjectConfig == nil {
		t.Fatalf("first syscap callback did not register session data: %#v", conn.requests[5].Args)
	}
	second, ok := conn.requests[6].Args[0].(clientOhSdkClassCheckRequest)
	if !ok || second.Node.Source != nil || second.Declaration.Source != nil || second.ProjectConfig != nil {
		t.Fatalf("second syscap callback repeated session data: %#v", conn.requests[6].Args)
	}
	if first.Node.SourceID != second.Node.SourceID || first.ProjectConfigID != second.ProjectConfigID {
		t.Fatalf("syscap session ids changed: first %#v, second %#v", first, second)
	}
}

func TestClientOhSdkPluginExecutorKeepsEveryRequestInAnOrderedBatch(t *testing.T) {
	conn := &sdkPluginCallbackConn{}
	executor := newClientOhSdkPluginExecutor()
	executor.ctx = context.Background()
	executor.conn = conn
	responses := []chan clientOhSdkPluginCallResult{
		make(chan clientOhSdkPluginCallResult, 1),
		make(chan clientOhSdkPluginCallResult, 1),
	}
	executor.executeBatch([]clientOhSdkPluginCall{
		{
			request:  ohSdkPluginRequest{Operation: "value", FunctionName: "value"},
			response: responses[0],
		},
		{
			request:  ohSdkPluginRequest{Operation: "regex", FunctionName: "regex"},
			response: responses[1],
		},
	})

	first := <-responses[0]
	second := <-responses[1]
	if first.err != nil || !first.response.Result || first.response.Message != "value" {
		t.Fatalf("first response = %#v", first)
	}
	if second.err != nil || !second.response.Matched || len(second.response.Groups) != 5 {
		t.Fatalf("second response = %#v", second)
	}
	if len(conn.requests) != 2 ||
		conn.requests[0].Operation != "value" ||
		conn.requests[1].Operation != "regex" {
		t.Fatalf("batch requests = %#v", conn.requests)
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

func TestClientOhSdkPluginExecutorPreservesFunctionInvocationFailurePolicy(t *testing.T) {
	executor := newClientOhSdkPluginExecutor()
	executor.SetConnection(context.Background(), sdkPluginInvokeErrorConn{})
	plugin := core.OhSdkCheckPlugin{Path: "/throwing.cjs", FunctionName: "check"}
	if _, found, err := executor.CheckValue(plugin, "1", "1", 0); err != nil || found {
		t.Fatalf("value invocation failure found %v, error %v", found, err)
	}
	if _, found, err := executor.CheckFormat(plugin, "1"); err != nil || found {
		t.Fatalf("format invocation failure found %v, error %v", found, err)
	}
}

type sdkPluginErrorConn struct{}

func (sdkPluginErrorConn) Run(context.Context) error                 { return nil }
func (sdkPluginErrorConn) Notify(context.Context, string, any) error { return nil }
func (sdkPluginErrorConn) Call(context.Context, string, any) (json.Value, error) {
	return json.Marshal([]ohSdkPluginResponse{{Phase: "load", Error: "module not found"}})
}

type sdkPluginInvokeErrorConn struct{}

func (sdkPluginInvokeErrorConn) Run(context.Context) error                 { return nil }
func (sdkPluginInvokeErrorConn) Notify(context.Context, string, any) error { return nil }
func (sdkPluginInvokeErrorConn) Call(context.Context, string, any) (json.Value, error) {
	return json.Marshal([]ohSdkPluginResponse{{Found: true, Phase: "invoke", Error: "callback threw"}})
}
