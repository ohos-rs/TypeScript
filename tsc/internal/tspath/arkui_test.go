package tspath_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
)

func TestArkUIExtensions(t *testing.T) {
	for _, tc := range []struct {
		name, extension string
		declaration     bool
	}{
		{"/Page.ets", ".ets", false},
		{"/sdk.d.ets", ".d.ets", true},
	} {
		if core.GetScriptKindFromFileName(tc.name) != core.ScriptKindETS || !tspath.HasTSFileExtension(tc.name) {
			t.Errorf("unrecognized ETS file: %s", tc.name)
		}
		if tspath.TryGetExtensionFromPath(tc.name) != tc.extension || tspath.TryExtractTSExtension(tc.name) != tc.extension {
			t.Errorf("wrong extension: %s", tc.name)
		}
		if tspath.IsDeclarationFileName(tc.name) != tc.declaration {
			t.Errorf("wrong declaration kind: %s", tc.name)
		}
		if tspath.GetDeclarationEmitExtensionForPath(tc.name) != ".d.ets" {
			t.Errorf("wrong declaration output: %s", tc.name)
		}
	}
	if tspath.RemoveFileExtension("/sdk.d.ets") != "/sdk" {
		t.Error("did not remove compound extension")
	}
	if core.GetDefaultExtensionForScriptKind(core.ScriptKindETS) != ".ets" {
		t.Error("wrong default ETS extension")
	}
}
