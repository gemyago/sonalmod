package workspacefs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetFileInfo(t *testing.T) {
	t.Parallel()

	t.Run("file_inside_root", func(t *testing.T) {
		t.Parallel()
		fname := faker.New().UUID().V4() + ".bin"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("hello"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: fname})
		require.NoError(t, err)
		assert.Equal(t, int64(5), res.Size)
		assert.Equal(t, fname, res.Name)
		assert.False(t, res.IsDirectory)
		assert.NotEmpty(t, res.ModTime)
		_, perr := time.Parse(time.RFC3339Nano, res.ModTime)
		require.NoError(t, perr)
	})

	t.Run("directory_inside_root", func(t *testing.T) {
		t.Parallel()
		nested := faker.New().UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, nested), 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: nested})
		require.NoError(t, err)
		assert.True(t, res.IsDirectory)
		assert.Equal(t, nested, res.Name)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()
		missing := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: missing})
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("rejects_absolute_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		bad := filepath.Join(abs, faker.New().UUID().V4()+".txt")
		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: bad})
		require.Error(t, err)
		assert.ErrorContains(t, err, "relative")
	})

	t.Run("rejects_path_outside_root_via_dotdot", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: "../../etc/passwd"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "escapes")
	})

	t.Run("empty_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: "  "})
		require.Error(t, err)
		assert.ErrorContains(t, err, "path is required")
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
		_, err = svc.GetFileInfo(ctx, GetFileInfoRequest{Workspace: "w", Path: "x"})
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

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Path: faker.New().UUID().V4() + ".txt"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("workspace_required_even_with_single_workspace", func(t *testing.T) {
		t.Parallel()
		fname := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("x"), 0o644))
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Path: fname})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Path: p, Workspace: "not-configured"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("stat_fails_with_permission", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		lockedName := fake.UUID().V4()
		inner := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		locked := filepath.Join(root, lockedName)
		require.NoError(t, os.Mkdir(locked, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(locked, inner), []byte("x"), 0o644))
		require.NoError(t, os.Chmod(locked, 0o000))
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.GetFileInfo(t.Context(), GetFileInfoRequest{Workspace: "w", Path: lockedName + "/" + inner})
		if err == nil {
			t.Skip("filesystem allowed stat despite parent mode 000")
		}
		require.Error(t, err)
		assert.ErrorContains(t, err, "stat")
	})
}
