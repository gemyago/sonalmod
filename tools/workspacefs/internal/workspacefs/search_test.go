package workspacefs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFiles(t *testing.T) {
	t.Parallel()

	t.Run("simple_glob_matches_files", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fa := fake.UUID().V4() + ".txt"
		fb := fake.UUID().V4() + ".txt"
		fc := fake.UUID().V4() + ".go"
		nested := fake.UUID().V4()
		fd := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fa), []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, fb), []byte("b"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, fc), []byte("c"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, nested), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, nested, fd), []byte("d"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "*.txt"})
		require.NoError(t, err)
		require.Len(t, res.Paths, 2)
		got := append([]string(nil), res.Paths...)
		sort.Strings(got)
		want := []string{fa, fb}
		sort.Strings(want)
		assert.Equal(t, want, got)
	})

	t.Run("no_matches_returns_empty_paths", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		only := fake.UUID().V4() + ".go"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, only), []byte("x"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "*.txt"})
		require.NoError(t, err)
		assert.Empty(t, res.Paths)
	})

	t.Run("rejects_path_escape", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: "../outside", Glob: "*.txt"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "escapes")
	})

	t.Run("rejects_glob_escape", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "../outside/*.txt"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "escapes")
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		missing := faker.New().UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: missing, Glob: "*"})
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("not_a_directory", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("x"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: fname, Glob: "*"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not a directory")
	})

	t.Run("empty_glob", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "  "})
		require.Error(t, err)
		assert.ErrorContains(t, err, "glob is required")
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = svc.SearchFiles(ctx, SearchFilesRequest{Workspace: "w", Path: ".", Glob: "*"})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("exceeds_max_list_entries", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		prefix := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		for i := range 4 {
			name := fmt.Sprintf("%s-%d.txt", prefix, i)
			require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644))
		}

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxListEntries(3),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "*.txt"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds maximum entries")
	})

	t.Run("workspace_required_when_multiple_workspaces", func(t *testing.T) {
		t.Parallel()
		r1 := t.TempDir()
		r2 := t.TempDir()
		a1, err := filepath.Abs(r1)
		require.NoError(t, err)
		a2, err := filepath.Abs(r2)
		require.NoError(t, err)

		svc, err := NewService([]WorkspaceMount{
			{Identifier: "a", Description: "test", Path: filepath.Clean(a1)},
			{Identifier: "b", Description: "test", Path: filepath.Clean(a2)},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Path: ".", Glob: "*"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("workspace_required_even_with_single_workspace", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("a"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Path: ".", Glob: "*.txt"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Path: ".", Glob: "*", Workspace: "bogus"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("invalid_glob_syntax", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("a"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "[unclosed"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "match")
	})

	t.Run("glob_must_be_relative", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: ".", Glob: "/abs/pattern"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "relative")
	})

	t.Run("stat_non_not_exist", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		badName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		bad := filepath.Join(root, badName)
		require.NoError(t, os.Mkdir(bad, 0o755))
		require.NoError(t, os.Chmod(bad, 0))
		t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.SearchFiles(t.Context(), SearchFilesRequest{Workspace: "w", Path: badName, Glob: "*"})
		if err == nil {
			t.Skip("filesystem allowed stat despite directory mode 000")
		}
		require.Error(t, err)
		msg := err.Error()
		ok := strings.Contains(msg, "stat") ||
			strings.Contains(msg, "read directory") ||
			strings.Contains(msg, "permission denied")
		assert.True(t, ok, "expected stat, read-directory, or permission error, got: %v", err)
	})
}

func TestGlobWalkFiles_cancelledContext(t *testing.T) {
	t.Parallel()
	fake := faker.New()
	key := fake.UUID().V4() + ".txt"
	svc := &Service{maxListEntries: DefaultMaxListEntries}
	fsys := fstest.MapFS{
		key: &fstest.MapFile{Data: []byte("x")},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.globWalkFiles(ctx, fsys, ".", "*.txt")
	require.ErrorIs(t, err, context.Canceled)
}

func TestRelPathUnderBase(t *testing.T) {
	t.Parallel()
	t.Run("full_under_base", func(t *testing.T) {
		t.Parallel()
		rel, ok := relPathUnderBase(".", "a/b.txt")
		require.True(t, ok)
		assert.Equal(t, "a/b.txt", rel)
	})
	t.Run("nested_base", func(t *testing.T) {
		t.Parallel()
		rel, ok := relPathUnderBase("foo", "foo/bar.txt")
		require.True(t, ok)
		assert.Equal(t, "bar.txt", rel)
	})
	t.Run("base_file_vs_full_mismatch", func(t *testing.T) {
		t.Parallel()
		_, ok := relPathUnderBase("foo", "bar")
		assert.False(t, ok)
	})
	t.Run("full_equals_base", func(t *testing.T) {
		t.Parallel()
		rel, ok := relPathUnderBase("foo", "foo")
		require.True(t, ok)
		assert.Equal(t, ".", rel)
	})
}
