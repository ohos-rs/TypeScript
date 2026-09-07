package parser_test

import (
	"encoding/json"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
)

// OH parser.ts::isCurrentTokenAnEtsComponentExpression and
// parseAssignmentExpressionOrHigher distinguish registered components from
// call-plus-block syntax. No capitalization or arbitrary render-name fallback.
func TestEtsConfiguredContexts(t *testing.T) {
	for _, tc := range []struct {
		name, config, source string
		components           int
		invalid              bool
	}{
		{"lowercase registration", `{"components":["widget"]}`, `struct S { build() { widget() {} } }`, 1, false},
		{"unregistered uppercase", `{}`, `struct S { build() { Widget() {} } }`, 0, true},
		{"configured generic call", `{"render":{"method":["build"]}}`, `struct S { build() { widget() {} } }`, 1, false},
		{"registered expression", `{"components":["widget"]}`, `struct S { build() { const x = widget(); } }`, 1, false},
		{"page transition", `{"components":["widget"]}`, `struct S { pageTransition() { widget() {} } }`, 1, false},
		{"arbitrary render excluded", `{"components":["widget"],"render":{"method":["render"]}}`, `struct S { render() { widget(); } }`, 0, false},
		{"builder fallback", `{"components":["widget"]}`, `@Builder function f() { widget() {} }`, 1, false},
		{"explicit empty builder", `{"components":["widget"],"render":{"decorator":[]}}`, `@Builder function f() { widget() {} }`, 0, true},
		{"renamed builder", `{"render":{"decorator":["Paint"]}}`, `@Paint function f() { widget() {} }`, 1, false},
		{"called builder excluded", `{"render":{"decorator":["Paint"]}}`, `@Paint() function f() { widget() {} }`, 0, true},
		{"ordinary callback isolated", `{"components":["widget"],"render":{"method":["build"]}}`, `struct S { build() { use(() => { widget(); }); } }`, 0, false},
		{"configured callback", `{"components":["widget"],"render":{"method":["build"]},"syntaxComponents":{"paramsUICallback":["RepeatItems"]}}`, `struct S { build() { RepeatItems([], () => { widget() {} }); } }`, 1, false},
		{"first callback is data", `{"components":["widget"],"render":{"method":["build"]},"syntaxComponents":{"paramsUICallback":["RepeatItems"]}}`, `struct S { build() { RepeatItems(() => { widget(); }, () => {}); } }`, 0, false},
		{"attribute callback", `{"components":["widget"],"render":{"method":["build"]},"syntaxComponents":{"attrUICallback":[{"name":"Collection","attributes":["each"]}]}}`, `struct S { build() { Collection().each(() => { widget() {} }); } }`, 1, false},
		{"first attribute record wins", `{"components":["widget"],"render":{"method":["build"]},"syntaxComponents":{"attrUICallback":[{"name":"Collection","attributes":[]},{"name":"Collection","attributes":["each"]}]}}`, `struct S { build() { Collection().each(() => { widget(); }); } }`, 0, false},
		{"unmatched extend excludes styles", `{"extend":{"decorator":["Extra"],"components":[]},"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `@Extra(Missing) @Paint function f() { .width(1) }`, 0, true},
		{"extend without argument excludes styles", `{"extend":{"decorator":["Extra"],"components":[]},"styles":{"decorator":"Paint","component":{"name":"Base","type":"T","instance":"BaseInstance"}}}`, `@Extra() @Paint function f() { .width(1) }`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config core.EtsOptions
			if err := json.Unmarshal([]byte(tc.config), &config); err != nil {
				t.Fatal(err)
			}
			file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/test.ets", Path: "/test.ets", Ets: config}, tc.source, core.ScriptKindETS)
			if (len(file.Diagnostics()) != 0) != tc.invalid {
				t.Fatalf("diagnostics: %v", file.Diagnostics())
			}
			count := 0
			var visit func(*ast.Node) bool
			visit = func(node *ast.Node) bool {
				if ast.IsEtsComponentExpression(node) {
					count++
				}
				node.ForEachChild(visit)
				return false
			}
			visit(file.AsNode())
			if count != tc.components {
				t.Fatalf("expected %d components, got %d", tc.components, count)
			}
		})
	}
}

func TestEtsConfiguredStyleNodes(t *testing.T) {
	var config core.EtsOptions
	if err := json.Unmarshal([]byte(`{"styles":{"decorator":"Paint","component":{"name":"Base","type":"Result","instance":"StyleReceiver"}},"extend":{"decorator":["Extra"],"components":[{"name":"widget","type":"WidgetStyle","instance":"WidgetReceiver"}]}}`), &config); err != nil {
		t.Fatal(err)
	}
	file := parser.ParseSourceFile(ast.SourceFileParseOptions{FileName: "/test.ets", Path: "/test.ets", Ets: config}, `@Paint function paint() { .width(1) } @Extra(widget) function extra() { .height(2) }`, core.ScriptKindETS)
	if len(file.Diagnostics()) != 0 {
		t.Fatal(file.Diagnostics())
	}
	for i, tc := range []struct {
		typ, receiver string
		generic       bool
	}{{"Result", "StyleReceiver", true}, {"WidgetStyle", "WidgetReceiver", false}} {
		fn := file.Statements.Nodes[i]
		if fn.Type().AsTypeReferenceNode().TypeName.Text() != tc.typ {
			t.Fatal("lost configured return type")
		}
		if (fn.TypeParameterList() != nil) != tc.generic {
			t.Fatal("wrong virtual type parameters")
		}
		call := fn.Body().Statements()[0].Expression()
		if call.Expression().Expression().Text() != tc.receiver {
			t.Fatal("receiver was guessed instead of using instance")
		}
	}
}
