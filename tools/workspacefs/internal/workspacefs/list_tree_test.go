package workspacefs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListDirectory(t *testing.T) {
	t.Parallel()

	t.Run("shallow_list_sorted_names_and_directory_flag", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		first := "a-" + fake.UUID().V4() + ".txt"
		last := "z-" + fake.UUID().V4() + ".txt"
		nestedDir := "m-" + fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, last), []byte("z"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, first), []byte("a"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, nestedDir), 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: "."})
		require.NoError(t, err)
		require.Len(t, res.Entries, 3)
		assert.Equal(t, first, res.Entries[0].Name)
		assert.False(t, res.Entries[0].IsDirectory)
		assert.Equal(t, nestedDir, res.Entries[1].Name)
		assert.True(t, res.Entries[1].IsDirectory)
		assert.Equal(t, last, res.Entries[2].Name)
		assert.False(t, res.Entries[2].IsDirectory)
	})

	t.Run("empty_directory", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		emptyName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		emptyDir := filepath.Join(root, emptyName)
		require.NoError(t, os.Mkdir(emptyDir, 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: emptyName})
		require.NoError(t, err)
		assert.Empty(t, res.Entries)
	})

	t.Run("not_a_directory", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		only := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, only), []byte("x"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: only})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not a directory")
	})

	t.Run("rejects_path_escape", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: "../outside"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "escapes")
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Path: ".", Workspace: "bogus"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("read_dir_permission_denied", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		lockedName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		locked := filepath.Join(root, lockedName)
		require.NoError(t, os.Mkdir(locked, 0o755))
		require.NoError(t, os.Chmod(locked, 0))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: lockedName})
		if err == nil {
			t.Skip("filesystem allowed listing directory despite mode 000")
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, "read directory")
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
		_, err = svc.ListDirectory(ctx, ListDirectoryRequest{Workspace: "w", Path: "."})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		missing := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: missing})
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("exceeds_max_list_entries", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		prefix := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		for i := range 5 {
			name := fmt.Sprintf("%s-%d.txt", prefix, i)
			require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644))
		}

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxListEntries(3),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Workspace: "w", Path: "."})
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

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Path: "."})
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

		_, err = svc.ListDirectory(t.Context(), ListDirectoryRequest{Path: "."})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})
}

func TestDirectoryTree(t *testing.T) {
	t.Parallel()

	t.Run("tree_depth_limit_stops_recursion", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		rootFile := fake.UUID().V4() + ".txt"
		dirA := fake.UUID().V4()
		a1 := fake.UUID().V4() + ".txt"
		dirB := fake.UUID().V4()
		deepFile := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, rootFile), []byte("r"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, dirA), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, dirA, a1), []byte("a1"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, dirA, dirB), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, dirA, dirB, deepFile), []byte("d"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		depth1 := 1
		res1, err := svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &depth1})
		require.NoError(t, err)
		require.NotNil(t, res1.Root)
		assert.Equal(t, ".", res1.Root.Name)
		require.Len(t, res1.Root.Children, 2)
		var aNode *DirectoryTreeNode
		for _, c := range res1.Root.Children {
			if c.Name == dirA {
				aNode = c
				break
			}
		}
		require.NotNil(t, aNode)
		assert.True(t, aNode.IsDirectory)
		assert.Empty(t, aNode.Children)

		depth2 := 2
		res2, err := svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &depth2})
		require.NoError(t, err)
		require.NotNil(t, res2.Root)
		var a2 *DirectoryTreeNode
		for _, c := range res2.Root.Children {
			if c.Name == dirA {
				a2 = c
				break
			}
		}
		require.NotNil(t, a2)
		require.Len(t, a2.Children, 2)
		var bNode *DirectoryTreeNode
		for _, c := range a2.Children {
			if c.Name == dirB {
				bNode = c
				break
			}
		}
		require.NotNil(t, bNode)
		assert.True(t, bNode.IsDirectory)
		assert.Empty(t, bNode.Children)

		depth3 := 3
		res3, err := svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &depth3})
		require.NoError(t, err)
		var a3 *DirectoryTreeNode
		for _, c := range res3.Root.Children {
			if c.Name == dirA {
				a3 = c
				break
			}
		}
		require.NotNil(t, a3)
		var b3 *DirectoryTreeNode
		for _, c := range a3.Children {
			if c.Name == dirB {
				b3 = c
				break
			}
		}
		require.NotNil(t, b3)
		require.Len(t, b3.Children, 1)
		assert.Equal(t, deepFile, b3.Children[0].Name)
		assert.False(t, b3.Children[0].IsDirectory)
	})

	t.Run("empty_directory_tree", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		emptyName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		empty := filepath.Join(root, emptyName)
		require.NoError(t, os.Mkdir(empty, 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 2
		res, err := svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: emptyName, MaxDepth: &d})
		require.NoError(t, err)
		require.NotNil(t, res.Root)
		assert.Equal(t, emptyName, res.Root.Name)
		assert.True(t, res.Root.IsDirectory)
		assert.Empty(t, res.Root.Children)
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

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: fname, MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not a directory")
	})

	t.Run("max_depth_below_one", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		bad := 0
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &bad})
		require.Error(t, err)
		assert.ErrorContains(t, err, "max_depth")
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		missing := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: missing, MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("omitted_max_depth_uses_default", func(t *testing.T) {
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

		res, err := svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: "."})
		require.NoError(t, err)
		require.NotNil(t, res.Root)
		require.Len(t, res.Root.Children, 1)
		assert.Equal(t, fname, res.Root.Children[0].Name)
	})

	t.Run("max_depth_clamped_to_hard_cap", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		deepName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, deepName), 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		deep := 500
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: deepName, MaxDepth: &deep})
		require.NoError(t, err)
	})

	t.Run("exceeds_max_list_entries", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		manyDir := fake.UUID().V4()
		prefix := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, manyDir), 0o755))
		for i := range 5 {
			name := fmt.Sprintf("%s-%d.txt", prefix, i)
			require.NoError(t, os.WriteFile(filepath.Join(root, manyDir, name), []byte("x"), 0o644))
		}

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxListEntries(4),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 2
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: manyDir, MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds maximum entries")
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
		d := 1
		_, err = svc.DirectoryTree(ctx, DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
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

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Path: ".", MaxDepth: &d})
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

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Path: ".", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("rejects_path_escape", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: "../outside", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "escapes")
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Path: ".", Workspace: "bogus", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("read_dir_permission_denied", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		lockedName := fake.UUID().V4()
		inner := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		locked := filepath.Join(root, lockedName)
		require.NoError(t, os.Mkdir(locked, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(locked, inner), []byte("x"), 0o644))
		require.NoError(t, os.Chmod(locked, 0))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 2
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: lockedName, MaxDepth: &d})
		if err == nil {
			t.Skip("filesystem allowed tree despite directory mode 000")
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, "read directory")
	})

	t.Run("bump_exhausted_after_root_and_files", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fa := fake.UUID().V4() + ".txt"
		fb := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fa), []byte("a"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, fb), []byte("b"), 0o644))

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxListEntries(2),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds maximum entries")
	})

	t.Run("bump_exhausted_on_shallow_subdirectory", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		subName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, subName), 0o755))

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxListEntries(1),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: ".", MaxDepth: &d})
		require.Error(t, err)
		assert.ErrorContains(t, err, "exceeds maximum entries")
	})

	t.Run("stat_non_not_exist", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		badName := fake.UUID().V4()
		inner := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		bad := filepath.Join(root, badName)
		require.NoError(t, os.Mkdir(bad, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(bad, inner), []byte("x"), 0o644))
		require.NoError(t, os.Chmod(bad, 0))
		t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		d := 1
		_, err = svc.DirectoryTree(t.Context(), DirectoryTreeRequest{Workspace: "w", Path: badName, MaxDepth: &d})
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
