package api

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/projecttestutil"
	"gotest.tools/v3/assert"
)

func TestArkTSTransformTypeFactsAPI(t *testing.T) {
	const file = "/entry.ets"
	ps, _ := projecttestutil.Setup(map[string]any{
		file: `class MutableBuilder { builder(): void {} }
type PrimitiveAlias = string | number;
type BuilderAlias = MutableBuilder;
enum Color { Red }
class Holder {
  simple: PrimitiveAlias = "value";
  content: BuilderAlias = new MutableBuilder();
  build() { this.content.builder(); const color = Color.Red; }
}`,
	})
	defer ps.Close()
	s := NewLSPSession(ps, nil)
	defer s.Close()
	ctx := t.Context()
	created, err := s.handleCreateProgram(ctx, &CreateProgramParams{
		RootFiles: []DocumentIdentifier{{FileName: file}},
		CreateProgramOptions: CreateProgramOptions{CompilerOptions: core.CompilerOptions{
			NoEmit:               core.TSTrue,
			EtsAnnotationsEnable: core.TSTrue,
		}},
	})
	assert.NilError(t, err)
	infos, err := s.handleGetArkTSTransformTypeFacts(ctx, &SelectedFilesEmitParams{
		Snapshot: created.Snapshot,
		Project:  created.Project.Id,
		Files:    []DocumentIdentifier{{FileName: file}},
	})
	assert.NilError(t, err)
	assert.Equal(t, len(infos), 1)
	assert.Equal(t, len(infos[0].Properties), 2)
	assert.Assert(t, len(infos[0].Properties[0].Type.Types) == 2)
	assert.Assert(t, infos[0].Properties[0].Type.Types[0].IsBasic)
	assert.Assert(t, infos[0].Properties[0].Type.Types[1].IsBasic)
	assert.Equal(t, len(infos[0].BuilderAccesses), 1)
	assert.Equal(t, infos[0].BuilderAccesses[0].ReceiverType.SymbolName, "MutableBuilder")
	assert.Equal(t, len(infos[0].MemberAccesses), 1)
	assert.Assert(t, infos[0].MemberAccesses[0].Type.IsEnum)
}
