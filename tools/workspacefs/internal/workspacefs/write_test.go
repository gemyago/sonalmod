package workspacefs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("request_JSON_round_trip_uses_workspace", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		in := WriteFileRequest{Path: p, Workspace: "w", Content: "x"}
		b, err := json.Marshal(in)
		require.NoError(t, err)
		assert.Contains(t, string(b), `"workspace":"w"`)
		assert.NotContains(t, string(b), `"root"`)
		var out WriteFileRequest
		require.NoError(t, json.Unmarshal(b, &out))
		assert.Equal(t, in, out)
	})

	t.Run("creates_new_file", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      fname,
			Content:   "first write",
		})
		require.NoError(t, err)
		assert.Equal(t, len("first write"), res.BytesWritten)

		b, err := os.ReadFile(filepath.Join(root, fname))
		require.NoError(t, err)
		assert.Equal(t, "first write", string(b))
	})

	t.Run("overwrites_existing_file", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		target := filepath.Join(root, fname)
		require.NoError(t, os.WriteFile(target, []byte("old"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      fname,
			Content:   "brand new",
		})
		require.NoError(t, err)
		assert.Equal(t, len("brand new"), res.BytesWritten)

		b, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "brand new", string(b))
	})

	t.Run("rejects_absolute_path", func(t *testing.T) {
		t.Parallel()
		x := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		bad := filepath.Join(abs, x)
		_, err = svc.WriteFile(t.Context(), WriteFileRequest{Workspace: "w", Path: bad, Content: "x"})
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

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      "../outside.txt",
			Content:   "no",
		})
		require.Error(t, err)
	})

	t.Run("rejects_empty_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{Workspace: "w", Path: "   ", Content: "x"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "path is required")
	})

	t.Run("rejects_invalid_utf8_content", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      p,
			Content:   string([]byte{0xff, 0xfe, 0xfd}),
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "UTF-8")
	})

	t.Run("creates_nested_file", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		d1 := fake.UUID().V4()
		d2 := fake.UUID().V4()
		out := fake.UUID().V4() + ".txt"
		rel := d1 + "/" + d2 + "/" + out
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      rel,
			Content:   "deep",
		})
		require.NoError(t, err)
		b, err := os.ReadFile(filepath.Join(root, d1, d2, out))
		require.NoError(t, err)
		assert.Equal(t, "deep", string(b))
	})

	t.Run("fails_when_target_is_existing_directory", func(t *testing.T) {
		t.Parallel()
		dironly := faker.New().UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, dironly), 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Workspace: "w",
			Path:      dironly,
			Content:   "x",
		})
		require.Error(t, err)
	})

	t.Run("multiple_workspaces_require_workspace_field", func(t *testing.T) {
		t.Parallel()
		fname := faker.New().UUID().V4() + ".txt"
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)

		svc, err := NewService([]WorkspaceMount{
			{Identifier: "a", Description: "test", Path: filepath.Clean(absA)},
			{Identifier: "b", Description: "test", Path: filepath.Clean(absB)},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{Path: fname, Content: "y"})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Path:      fname,
			Content:   "y",
			Workspace: "a",
		})
		require.NoError(t, err)
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = svc.WriteFile(ctx, WriteFileRequest{Workspace: "w", Path: p, Content: "a"})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{
			Path:      p,
			Content:   "y",
			Workspace: "not-configured",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("workspace_required_even_with_single_workspace", func(t *testing.T) {
		t.Parallel()
		p := faker.New().UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.WriteFile(t.Context(), WriteFileRequest{Path: p, Content: "y"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("no_workspaces_configured", func(t *testing.T) {
		t.Parallel()
		svc := &Service{maxReadBytes: DefaultMaxReadBytes}
		_, err := svc.WriteFile(t.Context(), WriteFileRequest{Workspace: "w", Path: "a", Content: "b"})
		require.Error(t, err)
		require.ErrorContains(t, err, "no workspaces configured")
	})
}
