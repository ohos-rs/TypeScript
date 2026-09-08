package api

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/ipc"
	"github.com/microsoft/TypeScript/tsc/internal/json"
)

const ohSdkPluginCallback = "ohSdkPlugin"

var ohSdkPluginSessionCounter atomic.Uint64

// ohSdkPluginRequest is the typed boundary between the checker and its host.
// The checker owns callback selection and diagnostic semantics. The API client
// owns the JavaScript runtime needed by SDK CommonJS plugin exports.
type ohSdkPluginRequest struct {
	SessionID    uint64 `json:"sessionId"`
	Path         string `json:"path"`
	FunctionName string `json:"functionName"`
	ClassName    string `json:"className,omitempty"`
	Operation    string `json:"operation"`
	Args         []any  `json:"args"`
}

type clientOhSdkPluginNodeSnapshot struct {
	SourceID uint32  `json:"sourceId"`
	FileName string  `json:"fileName"`
	Source   *string `json:"source,omitzero"`
	Pos      int     `json:"pos"`
	End      int     `json:"end"`
	Text     string  `json:"text"`
}

type clientOhSdkClassCheckRequest struct {
	Node            clientOhSdkPluginNodeSnapshot `json:"node"`
	Declaration     clientOhSdkPluginNodeSnapshot `json:"declaration"`
	ProjectConfigID uint32                        `json:"projectConfigId"`
	ProjectConfig   *map[string]any               `json:"projectConfig,omitzero"`
}

type clientOhSdkSource struct {
	id     uint32
	source string
}

type clientOhSdkProjectConfig struct {
	id     uint32
	config map[string]any
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
	ctx       context.Context
	conn      ipc.Conn
	sessionID uint64

	mu             sync.Mutex
	nextSourceID   uint32
	sources        map[string]clientOhSdkSource
	projectConfigs []clientOhSdkProjectConfig
}

func newClientOhSdkPluginExecutor() *clientOhSdkPluginExecutor {
	return &clientOhSdkPluginExecutor{
		sessionID: ohSdkPluginSessionCounter.Add(1),
		sources:   make(map[string]clientOhSdkSource),
	}
}

// SetConnection is called before the connection read loop starts. No checker
// request can observe a partially initialized executor.
func (e *clientOhSdkPluginExecutor) SetConnection(ctx context.Context, conn ipc.Conn) {
	e.ctx = ctx
	e.conn = conn
}

func (e *clientOhSdkPluginExecutor) call(plugin core.OhSdkCheckPlugin, operation string, args []any) (ohSdkPluginResponse, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.callLocked(plugin, operation, args)
}

func (e *clientOhSdkPluginExecutor) callLocked(plugin core.OhSdkCheckPlugin, operation string, args []any) (ohSdkPluginResponse, bool, error) {
	if e.conn == nil {
		return ohSdkPluginResponse{}, false, fmt.Errorf("OH SDK plugin callback invoked before API connection initialization")
	}
	request := ohSdkPluginRequest{
		SessionID: e.sessionID,
		Path:      plugin.Path, FunctionName: plugin.FunctionName,
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

func (e *clientOhSdkPluginExecutor) internSource(snapshot core.OhSdkPluginNodeSnapshot) clientOhSdkPluginNodeSnapshot {
	if cached, ok := e.sources[snapshot.FileName]; ok && cached.source == snapshot.Source {
		return clientOhSdkPluginNodeSnapshot{
			SourceID: cached.id, FileName: snapshot.FileName,
			Pos: snapshot.Pos, End: snapshot.End, Text: snapshot.Text,
		}
	}
	e.nextSourceID++
	id := e.nextSourceID
	e.sources[snapshot.FileName] = clientOhSdkSource{id: id, source: snapshot.Source}
	source := snapshot.Source
	return clientOhSdkPluginNodeSnapshot{
		SourceID: id, FileName: snapshot.FileName, Source: &source,
		Pos: snapshot.Pos, End: snapshot.End, Text: snapshot.Text,
	}
}

func (e *clientOhSdkPluginExecutor) internProjectConfig(config map[string]any) (uint32, *map[string]any) {
	for _, cached := range e.projectConfigs {
		if reflect.DeepEqual(cached.config, config) {
			return cached.id, nil
		}
	}
	id := uint32(len(e.projectConfigs) + 1)
	e.projectConfigs = append(e.projectConfigs, clientOhSdkProjectConfig{id: id, config: config})
	return id, &config
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
	e.mu.Lock()
	defer e.mu.Unlock()
	projectConfigID, projectConfig := e.internProjectConfig(request.ProjectConfig)
	wireRequest := clientOhSdkClassCheckRequest{
		Node:            e.internSource(request.Node),
		Declaration:     e.internSource(request.Declaration),
		ProjectConfigID: projectConfigID,
		ProjectConfig:   projectConfig,
	}
	response, found, err := e.callLocked(core.OhSdkCheckPlugin{
		Path: plugin.Path, FunctionName: plugin.ClassName,
	}, "syscap", []any{wireRequest})
	if err != nil && response.Phase == "load" {
		return core.OhSdkPluginSyscapResult{}, false, nil
	}
	return core.OhSdkPluginSyscapResult{
		CheckResult:  response.CheckResult,
		CheckMessage: response.CheckMessage,
	}, found, err
}
