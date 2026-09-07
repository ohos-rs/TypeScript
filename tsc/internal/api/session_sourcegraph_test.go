package api

import (
	"context"
	"testing"

	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/projecttestutil"
	"gotest.tools/v3/assert"
)

func TestProgramSourceGraphUsesResolvedModulesAndTypeReferences(t *testing.T) {
	t.Parallel()

	const root = "/home/projects/p/src/index.ts"
	projectSession, _ := projecttestutil.Setup(map[string]any{
		root:                           `import "./dep"; import "./leaf.js"; export const value = 1;`,
		"/home/projects/p/src/dep.ts":  `export const dep = 1;`,
		"/home/projects/p/src/leaf.js": `export const leaf = 1;`,
		"/home/projects/p/types/custom/index.d.ts": `declare const custom: string;`,
	})
	defer projectSession.Close()

	session := NewLSPSession(projectSession, nil)
	defer session.Close()
	response, err := session.handleCreateProgram(context.Background(), &CreateProgramParams{
		RootFiles: []DocumentIdentifier{{FileName: root}},
		CreateProgramOptions: CreateProgramOptions{CompilerOptions: core.CompilerOptions{
			NoLib:     core.TSTrue,
			AllowJs:   core.TSTrue,
			TypeRoots: []string{"/home/projects/p/types"},
			Types:     []string{"custom"},
		}},
	})
	assert.NilError(t, err)

	graph, err := session.handleGetProgramSourceGraph(context.Background(), &GetSourceFileNamesParams{
		Snapshot: response.Snapshot,
		Project:  response.Project.Id,
	})
	assert.NilError(t, err)
	assert.DeepEqual(t, graph.TypeReferenceFiles, []string{"/home/projects/p/types/custom/index.d.ts"})

	var rootDependencies []string
	for _, file := range graph.Files {
		if file.FileName == root {
			rootDependencies = file.Dependencies
			break
		}
	}
	assert.DeepEqual(t, rootDependencies, []string{"/home/projects/p/src/dep.ts"})
}
