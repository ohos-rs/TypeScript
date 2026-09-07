package project

import (
	"encoding/json"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
)

// OH sourceFileCompilerOptions changes grammar. The native cache must key the
// immutable options by content, including absent versus explicitly empty lists.
func TestEtsOptionsSeparateAndReuseParseCache(t *testing.T) {
	handle := newOverlay("/test.ets", "@Builder function f() { widget() {} }", 1, core.ScriptKindETS)
	cache := NewParseCache(RefCountCacheOptions{})
	var first *ast.SourceFile
	for i, raw := range []string{`{"components":["widget"]}`, `{"components":["widget"],"render":{"decorator":[]}}`, `{"render":{},"components":["widget"]}`} {
		var ets core.EtsOptions
		if err := json.Unmarshal([]byte(raw), &ets); err != nil {
			t.Fatal(err)
		}
		key := NewParseCacheKey(ast.SourceFileParseOptions{FileName: "/test.ets", Path: "/test.ets", Ets: ets}, handle.Hash(), handle.Kind())
		file := cache.Acquire(key, handle)
		defer cache.Deref(key)
		if (len(file.Diagnostics()) != 0) != (i == 1) {
			t.Fatal(file.Diagnostics())
		}
		if i == 0 {
			first = file
		} else if (file == first) != (i == 2) {
			t.Fatal("parse identity lost ETS content/presence semantics")
		}
	}
}
