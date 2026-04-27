package agentapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicSurfaceContract(t *testing.T) {
	t.Run("openapi contract uses profiles without opencode", func(t *testing.T) {
		spec := readRepoFile(t, "openapi.yaml")

		require.Contains(t, spec, "/agent-runs:")
		require.Contains(t, spec, "/sessions/{sessionId}/agent-runs:")
		require.Contains(t, spec, "AgentProfileExecutionSettings:")
		require.Contains(t, spec, "defaultModel:")
		require.Contains(t, spec, "const: acp-stdio")
		require.Contains(t, spec, "profileName:")

		require.NotContains(t, spec, "/opencode-bindings")
		require.NotContains(t, spec, "/opencode-launches")
		require.NotContains(t, spec, "OpenCode")
		require.NotContains(t, spec, "BindingName:")
	})

	t.Run("generated svelte client matches profile-only contract", func(t *testing.T) {
		client := readRepoFile(t, "../../../apps/sonal-ui/src/lib/agentapi/agentapi.generated.ts")

		require.Contains(t, client, "\"/agent-runs\":")
		require.Contains(t, client, "\"/sessions/{sessionId}/agent-runs\":")
		require.Contains(t, client, "AgentProfileExecutionSettings")
		require.Contains(t, client, "defaultModel: string;")
		require.Contains(t, client, "mode: \"acp-stdio\";")
		require.Contains(t, client, "profileName: string;")

		require.NotContains(t, client, "/opencode-bindings")
		require.NotContains(t, client, "/opencode-launches")
		require.NotContains(t, client, "OpenCode")
	})

	t.Run("public runtime packages expose no opencode symbols", func(t *testing.T) {
		assertNoExportedOpenCodeSymbols(t, "../../agent")
		assertNoExportedOpenCodeSymbols(t, "../../httpapi")
	})
}

func assertNoExportedOpenCodeSymbols(t *testing.T, relDir string) {
	t.Helper()

	pkgDir := repoPath(t, relDir)
	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(pkgDir, entry.Name())
		file, parseErr := parser.ParseFile(
			fset,
			path,
			nil,
			parser.SkipObjectResolution,
		)
		require.NoError(t, parseErr)

		ast.Inspect(file, func(node ast.Node) bool {
			switch decl := node.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					require.Falsef(t, isExportedOpenCodeName(decl.Name.Name), "%s exports %s", path, decl.Name.Name)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch typed := spec.(type) {
					case *ast.TypeSpec:
						require.Falsef(
							t,
							isExportedOpenCodeName(typed.Name.Name),
							"%s exports %s",
							path,
							typed.Name.Name,
						)
						assertNoExportedOpenCodeMembers(t, path, typed.Type)
					case *ast.ValueSpec:
						for _, name := range typed.Names {
							require.Falsef(
								t,
								isExportedOpenCodeName(name.Name),
								"%s exports %s",
								path,
								name.Name,
							)
						}
					}
				}
				return false
			}
			return true
		})
	}
}

func assertNoExportedOpenCodeMembers(t *testing.T, path string, expr ast.Expr) {
	t.Helper()

	switch typed := expr.(type) {
	case *ast.StructType:
		for _, field := range typed.Fields.List {
			for _, name := range field.Names {
				require.Falsef(
					t,
					isExportedOpenCodeName(name.Name),
					"%s exports field %s",
					path,
					name.Name,
				)
			}
		}
	case *ast.InterfaceType:
		for _, field := range typed.Methods.List {
			for _, name := range field.Names {
				require.Falsef(
					t,
					isExportedOpenCodeName(name.Name),
					"%s exports method %s",
					path,
					name.Name,
				)
			}
		}
	}
}

func isExportedOpenCodeName(name string) bool {
	return ast.IsExported(name) && strings.Contains(strings.ToLower(name), "opencode")
}

func readRepoFile(t *testing.T, relPath string) string {
	t.Helper()

	bytes, err := os.ReadFile(repoPath(t, relPath))
	require.NoError(t, err)

	return string(bytes)
}

func repoPath(t *testing.T, relPath string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)

	baseDir := filepath.Dir(file)
	cleaned := filepath.Clean(filepath.Join(baseDir, relPath))
	parts := strings.Split(filepath.ToSlash(cleaned), "/")
	require.Falsef(t, slices.Contains(parts, ".."), "resolved path escaped repository: %s", cleaned)

	return cleaned
}
