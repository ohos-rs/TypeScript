package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/microsoft/TypeScript/tsc/internal/core"
)

// The upstream SDK hooks are CommonJS modules loaded with require(). Keep the
// JavaScript host generic: Go owns selection and checker semantics, while Node
// only preserves the SDK's synchronous function and RegExp behavior.
const ohSdkPluginWorker = `
const readline = require('readline');
const path = require('path');
const { createRequire } = require('module');
const protocolWrite = process.stdout.write.bind(process.stdout);
process.stdout.write = function(chunk, encoding, callback) {
  return process.stderr.write(chunk, encoding, callback);
};
const send = value => protocolWrite(JSON.stringify(value) + '\n');
const classInstances = new Map();
const typeScriptModules = new Map();
const loadTypeScript = (pluginPath, etsLoaderPath) => {
  const key = etsLoaderPath || pluginPath;
  if (typeScriptModules.has(key)) return typeScriptModules.get(key);
  const pluginRequire = createRequire(pluginPath);
  const candidates = [];
  if (etsLoaderPath) candidates.push(path.join(etsLoaderPath, 'node_modules', 'typescript'));
  candidates.push('typescript');
  let lastError;
  for (const candidate of candidates) {
    try {
      const ts = candidate === 'typescript' ? pluginRequire(candidate) : require(candidate);
      typeScriptModules.set(key, ts);
      return ts;
    } catch (error) {
      lastError = error;
    }
  }
  throw lastError || new Error('Unable to load the SDK TypeScript module');
};
const locateNode = (ts, snapshot) => {
  const scriptKind = snapshot.fileName.endsWith('.ets') && ts.ScriptKind.ETS !== undefined
    ? ts.ScriptKind.ETS : ts.ScriptKind.TS;
  const sourceFile = ts.createSourceFile(
    snapshot.fileName, snapshot.source, ts.ScriptTarget.Latest, true, scriptKind
  );
  let exact;
  let containing;
  const visit = node => {
    if (node.pos > snapshot.pos || node.end < snapshot.end) return;
    if (!containing || node.end - node.pos < containing.end - containing.pos) containing = node;
    if ((node.pos === snapshot.pos || node.getStart(sourceFile) === snapshot.pos) &&
        node.end === snapshot.end && node.getText(sourceFile) === snapshot.text) {
      exact = node;
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFile);
  return exact || containing;
};
readline.createInterface({ input: process.stdin, crlfDelay: Infinity }).on('line', line => {
  let request;
  try {
    request = JSON.parse(line);
  } catch (error) {
    send({ id: 0, found: false, phase: 'protocol', error: String(error && error.stack || error) });
    return;
  }
  if (request.operation === 'prepareClass' || request.operation === 'syscap') {
    const instanceKey = request.path + '\0' + request.className;
    let checker = classInstances.get(instanceKey);
    try {
      if (!checker) {
        const CheckerClass = require(request.path)[request.className];
        if (typeof CheckerClass !== 'function') {
          send({ id: request.id, found: false });
          return;
        }
        checker = new CheckerClass();
        classInstances.set(instanceKey, checker);
      }
    } catch (error) {
      send({ id: request.id, found: false, phase: 'load', error: String(error && error.stack || error) });
      return;
    }
    if (request.operation === 'prepareClass') {
      send({ id: request.id, found: true });
      return;
    }
    try {
      if (typeof checker.check !== 'function') {
        send({ id: request.id, found: true, checkResult: false, checkMessage: '' });
        return;
      }
      const payload = request.args[0];
      const ts = loadTypeScript(request.path, payload.projectConfig.etsLoaderPath);
      const node = locateNode(ts, payload.node);
      const declaration = locateNode(ts, payload.declaration);
      if (!node || !declaration) {
        throw new Error('Unable to recreate SDK checker source nodes');
      }
      const projectConfig = {
        ...payload.projectConfig,
        syscapIntersectionSet: new Set(payload.projectConfig.syscapIntersection),
        syscapUnionSet: new Set(payload.projectConfig.syscapUnion),
        strictMode: { apiCompatibilityCheck: payload.projectConfig.apiCompatibilityCheck }
      };
      const value = checker.check(node, declaration, projectConfig);
      send({ id: request.id, found: true, checkResult: Boolean(value && value.checkResult),
        checkMessage: value && value.checkMessage == null ? '' : String(value.checkMessage) });
    } catch (error) {
      send({ id: request.id, found: true, phase: 'invoke', error: String(error && error.stack || error) });
    }
    return;
  }
  let callback;
  try {
    callback = require(request.path)[request.functionName];
  } catch (error) {
    send({ id: request.id, found: false, phase: 'load', error: String(error && error.stack || error) });
    return;
  }
  if (typeof callback !== 'function') {
    send({ id: request.id, found: false });
    return;
  }
  try {
    if (request.operation === 'regex') {
      const regex = callback();
      if (!(regex instanceof RegExp)) {
        send({ id: request.id, found: true, matched: false });
        return;
      }
      regex.lastIndex = 0;
      const match = regex.exec(request.args[0]);
      send({ id: request.id, found: true, matched: match !== null,
        groups: match === null ? [] : Array.from(match, value => value === undefined ? '' : String(value)) });
      return;
    }
    const value = callback(...request.args);
    if (request.operation === 'distribution') {
      send({ id: request.id, found: true, valid: Boolean(value && value.valid),
        version: value && value.version == null ? '' : String(value.version),
        message: value && value.message == null ? '' : String(value.message) });
      return;
    }
    send({ id: request.id, found: true, result: Boolean(value && value.result),
      message: value && value.message == null ? '' : String(value.message) });
  } catch (error) {
    send({ id: request.id, found: true, phase: 'invoke', error: String(error && error.stack || error) });
  }
});
`

type ohSdkPluginRequest struct {
	ID           uint64 `json:"id"`
	Path         string `json:"path"`
	FunctionName string `json:"functionName"`
	ClassName    string `json:"className,omitempty"`
	Operation    string `json:"operation"`
	Args         []any  `json:"args"`
}

type ohSdkPluginResponse struct {
	ID           uint64   `json:"id"`
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

type nodeOhSdkPluginExecutor struct {
	ctx            context.Context
	nodeExecutable string
	stderr         io.Writer

	mu     sync.Mutex
	child  *exec.Cmd
	input  io.WriteCloser
	output *bufio.Scanner
	nextID uint64
}

func newNodeOhSdkPluginExecutor(ctx context.Context, nodeExecutable string, stderr io.Writer) *nodeOhSdkPluginExecutor {
	return &nodeOhSdkPluginExecutor{ctx: ctx, nodeExecutable: nodeExecutable, stderr: stderr}
}

func (e *nodeOhSdkPluginExecutor) ensureStarted() error {
	if e.child != nil {
		return nil
	}
	command := exec.CommandContext(e.ctx, e.nodeExecutable, "-e", ohSdkPluginWorker)
	input, err := command.StdinPipe()
	if err != nil {
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	command.Stderr = e.stderr
	if err := command.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	e.child = command
	e.input = input
	e.output = scanner
	return nil
}

func (e *nodeOhSdkPluginExecutor) call(plugin core.OhSdkCheckPlugin, operation string, args []any) (ohSdkPluginResponse, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.ensureStarted(); err != nil {
		return ohSdkPluginResponse{}, false, err
	}
	e.nextID++
	request := ohSdkPluginRequest{
		ID: e.nextID, Path: plugin.Path, FunctionName: plugin.FunctionName,
		ClassName: plugin.FunctionName, Operation: operation, Args: args,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return ohSdkPluginResponse{}, false, err
	}
	if _, err := e.input.Write(append(encoded, '\n')); err != nil {
		return ohSdkPluginResponse{}, false, err
	}
	if !e.output.Scan() {
		if err := e.output.Err(); err != nil {
			return ohSdkPluginResponse{}, false, err
		}
		return ohSdkPluginResponse{}, false, fmt.Errorf("SDK plugin host exited before responding")
	}
	var response ohSdkPluginResponse
	if err := json.Unmarshal(e.output.Bytes(), &response); err != nil {
		return ohSdkPluginResponse{}, false, err
	}
	if response.ID != request.ID {
		return ohSdkPluginResponse{}, false, fmt.Errorf("SDK plugin response id %d does not match request %d", response.ID, request.ID)
	}
	if response.Error != "" && response.Found {
		return ohSdkPluginResponse{}, true, fmt.Errorf("SDK plugin %s#%s %s failed: %s", plugin.Path, plugin.FunctionName, response.Phase, response.Error)
	}
	return response, response.Found, nil
}

func (e *nodeOhSdkPluginExecutor) CheckValue(plugin core.OhSdkCheckPlugin, required string, target string, scene int) (core.OhSdkPluginCheckResult, bool, error) {
	response, found, err := e.call(plugin, "value", []any{required, target, scene})
	return core.OhSdkPluginCheckResult{Result: response.Result, Message: response.Message}, found, err
}

func (e *nodeOhSdkPluginExecutor) PrepareClass(plugin core.OhSdkClassCheckPlugin) (bool, error) {
	_, found, err := e.call(core.OhSdkCheckPlugin{
		Path: plugin.Path, FunctionName: plugin.ClassName,
	}, "prepareClass", nil)
	return found, err
}

func (e *nodeOhSdkPluginExecutor) CheckFormat(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginCheckResult, bool, error) {
	response, found, err := e.call(plugin, "format", []any{version})
	return core.OhSdkPluginCheckResult{Result: response.Result, Message: response.Message}, found, err
}

func (e *nodeOhSdkPluginExecutor) CheckDistribution(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginDistributionResult, bool, error) {
	response, found, err := e.call(plugin, "distribution", []any{version})
	return core.OhSdkPluginDistributionResult{Valid: response.Valid, Version: response.Version, Message: response.Message}, found, err
}

func (e *nodeOhSdkPluginExecutor) MatchBuildVersion(plugin core.OhSdkCheckPlugin, version string) (core.OhSdkPluginRegexResult, bool, error) {
	response, found, err := e.call(plugin, "regex", []any{version})
	return core.OhSdkPluginRegexResult{Matched: response.Matched, Groups: response.Groups}, found, err
}

func (e *nodeOhSdkPluginExecutor) CheckSyscap(plugin core.OhSdkClassCheckPlugin, request core.OhSdkClassCheckRequest) (core.OhSdkPluginSyscapResult, bool, error) {
	response, found, err := e.call(core.OhSdkCheckPlugin{
		Path: plugin.Path, FunctionName: plugin.ClassName,
	}, "syscap", []any{request})
	return core.OhSdkPluginSyscapResult{
		CheckResult:  response.CheckResult,
		CheckMessage: response.CheckMessage,
	}, found, err
}

func (e *nodeOhSdkPluginExecutor) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.child == nil {
		return nil
	}
	_ = e.input.Close()
	err := e.child.Wait()
	e.child = nil
	e.input = nil
	e.output = nil
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == -1 {
		return nil
	}
	return err
}
