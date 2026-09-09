package api

import (
	"strings"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/projecttestutil"
	"gotest.tools/v3/assert"
)

func TestArkTSTransformTypeFactsAPI(t *testing.T) {
	const file = "/entry.ets"
	const sdk = "/sdk/@ohos.sample.d.ts"
	const source = `class MutableBuilder { builder(): void {} }
type PrimitiveAlias = string | number;
type BuilderAlias = MutableBuilder;
enum Color { Red }
class Holder {
  simple: PrimitiveAlias = "value";
  content: BuilderAlias = new MutableBuilder();
  build() { this.content.builder(); const color = Color.Red; sample.create().run(); }
}`
	ps, _ := projecttestutil.Setup(map[string]any{
		sdk: `declare namespace sample {
/** @crossplatform */ interface Client { /** @crossplatform */ run(): void; }
/** @crossplatform */ function create(): Client;
}`,
		file: source,
	})
	defer ps.Close()
	s := NewLSPSession(ps, nil)
	defer s.Close()
	ctx := t.Context()
	created, err := s.handleCreateProgram(ctx, &CreateProgramParams{
		RootFiles: []DocumentIdentifier{{FileName: sdk}, {FileName: file}},
		CreateProgramOptions: CreateProgramOptions{CompilerOptions: core.CompilerOptions{
			NoEmit:               core.TSTrue,
			EtsAnnotationsEnable: core.TSTrue,
			OhCrossplatform:      core.TSTrue,
			OhAllModulePaths:     []string{sdk},
		}},
	})
	assert.NilError(t, err)
	rangeOf := func(text string) *ArkTSTypeQueryRange {
		pos := strings.Index(source, text)
		assert.Assert(t, pos >= 0)
		return &ArkTSTypeQueryRange{
			Pos: int(core.UTF16Len(source[:pos])),
			End: int(core.UTF16Len(source[:pos+len(text)])),
		}
	}
	infos, err := s.handleGetArkTSTransformTypeFacts(ctx, &ArkTSTransformTypeFactsParams{
		Snapshot: created.Snapshot,
		Project:  created.Project.Id,
		Queries: []*ArkTSTransformTypeFactsFileQuery{{
			File:            DocumentIdentifier{FileName: file},
			Properties:      []*ArkTSTypeQueryRange{rangeOf("simple"), rangeOf("content")},
			BuilderAccesses: []*ArkTSTypeQueryRange{rangeOf("this.content.builder")},
			MemberAccesses:  []*ArkTSTypeQueryRange{rangeOf("Color.Red")},
		}},
		SDKApiUseFiles: []DocumentIdentifier{{FileName: file}},
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
	enumMembers := 0
	for _, access := range infos[0].MemberAccesses {
		if access.Type.IsEnum {
			enumMembers++
		}
	}
	assert.Equal(t, enumMembers, 1)
	assert.Equal(t, len(infos[0].SDKApiUses), 3)
	assert.Equal(t, infos[0].SDKApiUses[0].ApiModule, "@ohos.sample")
	assert.Equal(t, infos[0].SDKApiUses[0].Function, "Client#run")
	assert.Equal(t, infos[0].SDKApiUses[1].Function, "create")
	assert.Equal(t, infos[0].SDKApiUses[2].Function, "sample")
}
