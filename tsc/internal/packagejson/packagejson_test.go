package packagejson_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/microsoft/TypeScript/tsc/internal/ast"
	"github.com/microsoft/TypeScript/tsc/internal/core"
	"github.com/microsoft/TypeScript/tsc/internal/json"
	"github.com/microsoft/TypeScript/tsc/internal/packagejson"
	"github.com/microsoft/TypeScript/tsc/internal/parser"
	"github.com/microsoft/TypeScript/tsc/internal/repo"
	"github.com/microsoft/TypeScript/tsc/internal/testutil/filefixture"
	"github.com/microsoft/TypeScript/tsc/internal/tspath"
	"gotest.tools/v3/assert"
)

var packageJsonFixtures = []filefixture.Fixture{
	filefixture.FromFile("package.json", filepath.Join(repo.RootPath(), "package.json")),
	filefixture.FromFile("date-fns.json", filepath.Join(repo.TestDataPath(), "fixtures", "packagejson", "date-fns.json")),
}

func TestParseJSON5PreservesConditionalExportOrder(t *testing.T) {
	t.Parallel()

	fields, err := packagejson.ParseJSON5([]byte(`{
		// The OH resolver uses the json5 package for this manifest.
		name: 'pkg',
		version: '1.0.0',
		types: 'index.d.ets',
		exports: {
			'.': {
				'types': './types.d.ets',
				'default': './index.js',
			},
		},
	}`))
	assert.NilError(t, err)
	name, ok := fields.Name.GetValue()
	assert.Assert(t, ok)
	assert.Equal(t, name, "pkg")
	root, ok := fields.Exports.AsObject().Get(".")
	assert.Assert(t, ok)
	assert.DeepEqual(t, slices.Collect(root.AsObject().Keys()), []string{"types", "default"})
}

func BenchmarkPackageJSON(b *testing.B) {
	for _, f := range packageJsonFixtures {
		f.SkipIfNotExist(b)
		content := []byte(f.ReadFile(b))
		b.Run("UnmarshalJSON", func(b *testing.B) {
			b.Run(f.Name(), func(b *testing.B) {
				for b.Loop() {
					var p packagejson.Fields
					if err := json.Unmarshal(content, &p); err != nil {
						b.Fatal(err)
					}
				}
			})
		})

		b.Run("UnmarshalJSONV2", func(b *testing.B) {
			b.Run(f.Name(), func(b *testing.B) {
				for b.Loop() {
					var p packagejson.Fields
					if err := json.Unmarshal(content, &p); err != nil {
						b.Fatal(err)
					}
				}
			})
		})

		b.Run("ParseJSONText", func(b *testing.B) {
			b.Run(f.Name(), func(b *testing.B) {
				fileName := "/" + f.Name()
				for b.Loop() {
					parser.ParseSourceFile(ast.SourceFileParseOptions{
						FileName: fileName,
						Path:     tspath.Path(fileName),
					}, string(content), core.ScriptKindJSON)
				}
			})
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    packagejson.Fields
	}{
		{
			name: "duplicate names",
			content: `{
				"name": "test-package",
				"name": "test-package",
				"version": "1.0.0"
			}`,
			want: packagejson.Fields{
				HeaderFields: packagejson.HeaderFields{
					Name:    packagejson.ExpectedOf("test-package"),
					Version: packagejson.ExpectedOf("1.0.0"),
				},
			},
		},
		{
			name: "content mapper",
			content: `{
				"name": "test-package",
				"typescript": {
					"contentMapper": { "exec": ["mapper"], "dynamicConfig": true }
				}
			}`,
			want: packagejson.Fields{
				HeaderFields: packagejson.HeaderFields{Name: packagejson.ExpectedOf("test-package")},
				ContentMapper: packagejson.ExpectedOf(packagejson.ContentMapperFields{
					Exec:          packagejson.ExpectedOf([]string{"mapper"}),
					DynamicConfig: packagejson.ExpectedOf(true),
				}),
			},
		},
		{
			name:    "invalid typescript field is ignored",
			content: `{ "name": "test-package", "typescript": "invalid" }`,
			want: packagejson.Fields{
				HeaderFields: packagejson.HeaderFields{Name: packagejson.ExpectedOf("test-package")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := packagejson.Parse([]byte(tt.content))
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tt.want, cmpopts.IgnoreUnexported(
				packagejson.Fields{},
				packagejson.HeaderFields{},
				packagejson.Expected[string]{},
				packagejson.Expected[bool]{},
				packagejson.Expected[map[string]string]{},
				packagejson.Expected[[]string]{},
				packagejson.Expected[packagejson.ContentMapperFields]{},
				packagejson.ExportsOrImports{},
			))
		})
	}
}
