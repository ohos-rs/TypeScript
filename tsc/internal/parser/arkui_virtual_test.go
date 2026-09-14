package parser_test

import (
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/etstest"
)

// OH parser.ts::parseStructMembers builds each virtual property signature with
// getModifiers(property), so the member's decorators survive into the injected
// constructor's parameter bag; and both branches that synthesize the readonly
// token call finishVirtualNode.
//
// Both facts are load-bearing: the build transform reads @Require off the
// signature, and Virtual is what keeps synthesized nodes out of missing-node
// checks, trailing-comma detection and the AST wire protocol.
func TestArkUIStructVirtualMembers(t *testing.T) {
	source := `struct Page {
	  @Require @Prop label: string;
	  @Param value: number = 0;
	  build() {}
	}`
	file := parser.ParseSourceFile(
		ast.SourceFileParseOptions{Ets: etstest.Options(), FileName: "/page.ets", Path: "/page.ets"},
		source,
		core.ScriptKindETS,
	)
	if ds := file.Diagnostics(); len(ds) != 0 {
		t.Fatalf("parse diagnostics: %v", ds)
	}

	var signatureDecorators, virtualReadonly, plainReadonly int
	var visit ast.Visitor
	visit = func(n *ast.Node) bool {
		switch n.Kind {
		case ast.KindPropertySignature:
			for _, modifier := range n.ModifierNodes() {
				if modifier.Kind == ast.KindDecorator {
					signatureDecorators++
				}
			}
		case ast.KindReadonlyKeyword:
			if n.Virtual {
				virtualReadonly++
			} else {
				plainReadonly++
			}
		}
		n.ForEachChild(visit)
		return false
	}
	visit(file.AsNode())

	// `label` carries @Require @Prop, `value` carries @Param.
	if signatureDecorators != 3 {
		t.Errorf("virtual property signature dropped decorators: got %d, want 3", signatureDecorators)
	}
	if virtualReadonly == 0 {
		t.Error("synthesized readonly modifier is not marked virtual")
	}
	if plainReadonly != 0 {
		t.Errorf("unexpected non-virtual readonly modifiers: %d", plainReadonly)
	}
}
