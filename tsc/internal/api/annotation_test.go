package api

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/projecttestutil"
	"gotest.tools/v3/assert"
)

// OH checker.ts annotation constant/default/type queries: keep declaration
// order, distinguish explicit [] from no initializer, and retain numeric bits.
func TestAnnotationInfoAPI(t *testing.T) {
	const file = "/home/projects/p/main.ets"
	ps, _ := projecttestutil.Setup(map[string]any{
		file: `
const enum Numeric { First = 7, Second = 9 }
const enum Words { First = "first", Second = "second" }
@interface Info {
  scalar: number = -0;
  positive: number = Infinity;
  invalidNumber: number = NaN;
  flag: boolean = false;
  text: string = "";
  nested: number[][] = [];
  numeric: Numeric;
  words: Words[][];
@Info({ text: "provided", scalar: 1 + 2, numeric: Numeric.Second, words: [] })
class Consumer {}
`,
	})
	defer ps.Close()
	s := NewLSPSession(ps, nil)
	defer s.Close()
	ctx := t.Context()
	created, err := s.handleCreateProgram(ctx, &CreateProgramParams{
		RootFiles:            []DocumentIdentifier{{FileName: file}},
		CreateProgramOptions: CreateProgramOptions{CompilerOptions: core.CompilerOptions{NoEmit: core.TSTrue, EtsAnnotationsEnable: core.TSTrue}},
	})
	assert.NilError(t, err)
	setup, err := s.setupChecker(ctx, created.Snapshot, created.Project.Id)
	assert.NilError(t, err)
	var annotation, use NodeHandle
	for _, statement := range setup.program.GetSourceFile(file).Statements.Nodes {
		if ast.IsAnnotationDeclaration(statement) {
			annotation = setup.sd.nodeHandleFrom(statement)
		}
		if ast.IsClassDeclaration(statement) && !ast.IsAnnotationDeclaration(statement) {
			use = setup.sd.nodeHandleFrom(statement.ModifierNodes()[0])
		}
	}
	setup.done()
	assert.Assert(t, annotation != "" && use != "")
	request := &CheckerNodeParams{Snapshot: created.Snapshot, Project: created.Project.Id, Location: annotation}
	info, err := s.handleGetAnnotationInfo(ctx, request)
	assert.NilError(t, err)
	assert.Assert(t, info != nil)
	assert.Equal(t, len(info.Properties), 8)
	for i, value := range []string{"-0", "Infinity", "NaN", "false", ""} {
		assert.Assert(t, info.Properties[i].Initializer != nil)
		assert.Equal(t, info.Properties[i].Initializer.Value, value)
	}
	assert.Equal(t, info.Properties[5].ArrayDepth, 2)
	assert.Equal(t, info.Properties[5].Initializer.Kind, "array")
	assert.Assert(t, info.Properties[5].Initializer.Items != nil)
	assert.Assert(t, info.Properties[6].Initializer == nil)
	assert.Assert(t, info.Properties[6].EnumDeclaration != "")
	assert.Equal(t, info.Properties[6].EnumFirstValue.Value, "7")
	assert.Equal(t, info.Properties[7].ArrayDepth, 2)
	assert.Equal(t, info.Properties[7].EnumFirstValue.Value, "first")
	request.Location = use
	used, err := s.handleGetAnnotationInfo(ctx, request)
	assert.NilError(t, err)
	assert.Assert(t, used != nil)
	assert.Equal(t, used.Declaration, annotation)
	assert.Equal(t, used.Properties[0].Name, "scalar")
	assert.Equal(t, used.Properties[0].Argument.Value, "3")
	assert.Equal(t, used.Properties[4].Argument.Value, "provided")
	assert.Equal(t, used.Properties[6].Argument.Value, "9")
	assert.Equal(t, used.Properties[7].Argument.Kind, "array")
	assert.Assert(t, used.Properties[1].Argument == nil)
}

func TestAnnotationTransformInfoAPI(t *testing.T) {
	const file = "/entry.ets"
	const source = `const marker = "😀";
import { Retention, RetentionPolicy } from './@arkts.lang';
import { Remote } from './runtime';
@Retention({policy: RetentionPolicy.SOURCE}) @interface Source { value = 42; }
@Source({value: 1}) function removed() {}
@Remote
class Consumer {
  @Remote value: number = 0;
  @Remote({value: 2}) method() {}
}
`
	ps, _ := projecttestutil.Setup(map[string]any{
		"/@arkts.lang.d.ets": `export declare const enum RetentionPolicy { SOURCE = "source" }
export declare @interface Retention { policy: RetentionPolicy; }`,
		"/runtime.d.ets": `export declare @interface Remote { value: number = 1; }`,
		file:             source,
	})
	defer ps.Close()
	s := NewLSPSession(ps, nil)
	defer s.Close()
	ctx := t.Context()
	created, err := s.handleCreateProgram(ctx, &CreateProgramParams{
		RootFiles:            []DocumentIdentifier{{FileName: file}},
		CreateProgramOptions: CreateProgramOptions{CompilerOptions: core.CompilerOptions{NoEmit: core.TSTrue, EtsAnnotationsEnable: core.TSTrue}},
	})
	assert.NilError(t, err)
	infos, err := s.handleGetAnnotationTransformInfos(ctx, &SelectedFilesEmitParams{
		Snapshot: created.Snapshot,
		Project:  created.Project.Id,
		Files:    []DocumentIdentifier{{FileName: file}},
	})
	assert.NilError(t, err)
	assert.Equal(t, len(infos), 1)
	info := infos[0]
	assert.Equal(t, info.FileName, file)
	assert.Equal(t, len(info.Declarations), 1)
	assert.Assert(t, info.Declarations[0].Info.SourceRetention)
	assert.Equal(t, info.Declarations[0].Info.Properties[0].TypeText, "number")
	assert.Equal(t, info.Declarations[0].Info.Properties[0].Initializer.Value, "42")
	assert.Equal(t, len(info.Uses), 5)
	retained := 0
	for _, use := range info.Uses {
		if use.RuntimeRetained {
			retained++
		}
	}
	assert.Equal(t, retained, 2)
	assert.Equal(t, len(info.Imports), 3)
	assert.Equal(t, info.Imports[0].Disposition, "remove")
	assert.Equal(t, info.Imports[1].Disposition, "remove")
	assert.Equal(t, info.Imports[2].Disposition, "rename")
	remoteUTF8 := strings.Index(source, "Remote } from")
	// ImportSpecifier.Pos includes the leading space; the preceding 😀 adds the
	// UTF-16 unit that offsets removing that one ASCII byte.
	remoteUTF16 := len([]rune(source[:remoteUTF8]))
	assert.Equal(t, info.Imports[2].Pos, remoteUTF16)
}
