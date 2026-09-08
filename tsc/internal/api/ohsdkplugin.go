package api

import (
	"context"
	"fmt"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/ipc"
	"github.com/microsoft/TypeScript/tsc/internal/json"
)

const ohSdkPluginCallback = "ohSdkPlugin"

// ohSdkPluginRequest is the typed boundary between the checker and its host.
// The checker owns callback selection and diagnostic semantics. The API client
// owns the JavaScript runtime needed by SDK CommonJS plugin exports.
type ohSdkPluginRequest struct {
	Path         string `json:"path"`
	FunctionName string `json:"functionName"`
	ClassName    string `json:"className,omitempty"`
	Operation    string `json:"operation"`
	Args         []any  `json:"args"`
}

type ohSdkPluginResponse struct {
	Found        bool     `json:"found"`
	Result       bool     `json:"result"`
	Message      string   `json:"message"`
	Valid        bool     `json:"valid"`
	Version      string   `json:"version"`
	Matched      bool     `json:"matched"`
	Groups       []string `json:"groups"`
	Phase        string   `json:"phase"`
	Error        string   `json:"error"`
	CheckResult  bool     `json:"checkResult"`
	CheckMessage string   `json:"checkMessage"`
}

// clientOhSdkPluginExecutor delegates the source-owned JavaScript plugin ABI
// to the API client. TSGO never starts Node or evaluates JavaScript.
type clientOhSdkPluginExecutor struct {
	ctx  context.Context
	conn ipc.Conn
}

func newClientOhSdkPluginExecutor() *clientOhSdkPluginExecutor {
	return &clientOhSdkPluginExecutor{}
}

// SetConnection is called before the connection read loop starts. No checker
// request can observe a partially initialized executor.
func (e *clientOhSdkPluginExecutor) SetConnection(ctx context.Context, conn ipc.Conn) {
	e.ctx = ctx
	e.conn = conn
}

func (e *clientOhSdkPluginExecutor) call(plugin core.OhSdkCheckPlugin, operation string, args []any) (ohSdkPluginResponse, bool, error) {
	if e.conn == nil {
		return ohSdkPluginResponse{}, false, fmt.Errorf("OH SDK plugin callback invoked before API connection initialization")
	}
	request := ohSdkPluginRequest{
		Path: plugin.Path, FunctionName: plugin.FunctionName,
		ClassName: plugin.FunctionName, Operation: operation, Args: args,
	}
	value, err := e.conn.Call(e.ctx, ohSdkPluginCallback, request)
	if err != nil {
		return ohSdkPluginResponse{}, false, err
	}
	var response ohSdkPluginResponse
	if err := json.Unmarshal(value, &response); err != nil {
		return ohSdkPluginResponse{}, false, fmt.Errorf("invalid OH SDK plugin response: %w", err)
	}
	if response.Error != "" {
		return response, response.Found, fmt.Errorf("SDK plugin %s#%s %s failed: %s", plugin.Path, plugin.FunctionName, response.Phase, response.Error)
	}
	return response, response.Found, nil
}

func (e *clientOhSdkPluginExecutor) CheckValue(plugin core.OhSdkCheckPlugin, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool, error) {
	response, found, err := e.call(plugin, "value", []any{required, target, scene})
	if err != nil && response.Phase == "load" {
		return core.OhSdkPluginCheckResult{}, false, nil
	}
	return core.OhSdkPluginCheckResult{Result: response.Result, Message: response.Message}, found, err
}

func (e *clientOhSdkPluginExecutor) PrepareClass(plugin core.OhSdkClassCheckPlugin) (bool, error) {
	_, found, err := e.call(core.OhSdkCheckPlugin{
		Path: plugin.Path, FunctionName: plugin.ClassName,
	}, "prepareClass", nil)
	if err != nil && !found {
		return false, nil
	}
	return found, err
}

func (e *clientOhSdkPluginExecutor) CheckFormat(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginCheckResult, bool, error) {
	response, found, err := e.call(plugin, "format", []any{version})
	if err != nil && response.Phase == "load" {
		return core.OhSdkPluginCheckResult{}, false, nil
	}
	return core.OhSdkPluginCheckResult{Result: response.Result, Message: response.Message}, found, err
}

func (e *clientOhSdkPluginExecutor) CheckDistribution(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginDistributionResult, bool, error) {
	response, found, err := e.call(plugin, "distribution", []any{version})
	return core.OhSdkPluginDistributionResult{Valid: response.Valid, Version: response.Version, Message: response.Message}, found, err
}

func (e *clientOhSdkPluginExecutor) MatchBuildVersion(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginRegexResult, bool, error) {
	response, found, err := e.call(plugin, "regex", []any{version})
	return core.OhSdkPluginRegexResult{Matched: response.Matched, Groups: response.Groups}, found, err
}

func (e *clientOhSdkPluginExecutor) CheckSyscap(plugin core.OhSdkClassCheckPlugin, request core.OhSdkClassCheckRequest) (core.OhSdkPluginSyscapResult, bool, error) {
	response, found, err := e.call(core.OhSdkCheckPlugin{
		Path: plugin.Path, FunctionName: plugin.ClassName,
	}, "syscap", []any{request})
	if err != nil && response.Phase == "load" {
		return core.OhSdkPluginSyscapResult{}, false, nil
	}
	return core.OhSdkPluginSyscapResult{
		CheckResult:  response.CheckResult,
		CheckMessage: response.CheckMessage,
	}, found, err
}
