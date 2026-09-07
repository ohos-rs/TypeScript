package project

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
)

// OH parser.ts::inAllowAnnotationContext requires an explicit host option.
// Reusing the same source text under another option must not reuse its AST.
func TestAnnotationOptionSeparatesParseCache(t *testing.T) {
	handle := newOverlay("/input.ets", "export @interface A {}", 1, core.ScriptKindETS)
	cache := NewParseCache(RefCountCacheOptions{})
	options := ast.SourceFileParseOptions{FileName: "/input.ets", Path: "/input.ets"}
	disabledKey := NewParseCacheKey(options, handle.Hash(), handle.Kind())
	disabled := cache.Acquire(disabledKey, handle)
	defer cache.Deref(disabledKey)
	if len(disabled.Diagnostics()) == 0 {
		t.Fatal("annotation grammar enabled without host option")
	}
	options.EtsAnnotationsEnable = true
	enabledKey := NewParseCacheKey(options, handle.Hash(), handle.Kind())
	enabled := cache.Acquire(enabledKey, handle)
	defer cache.Deref(enabledKey)
	if enabled == disabled || len(enabled.Diagnostics()) != 0 || !ast.IsAnnotationDeclaration(enabled.Statements.Nodes[0]) {
		t.Fatal("annotation option reused the disabled parse or lost annotation identity")
	}
	if again := cache.Acquire(enabledKey, handle); again != enabled {
		t.Fatal("unchanged parse options did not reuse the cached AST")
	}
	cache.Deref(enabledKey)
}
