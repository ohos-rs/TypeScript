package api

import (
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
}
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
