package workspacefs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditFile(t *testing.T) {
	t.Parallel()

	t.Run("replaces_first_occurrence_only", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		token := fake.Lorem().Word()
		replacement := fake.Lorem().Word()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		triple := strings.Join([]string{token, token, token}, " ")
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(triple), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   token,
			NewText:   replacement,
		})
		require.NoError(t, err)
		assert.Equal(t, 3, res.MatchCount)
		assert.Positive(t, res.BytesWritten)

		b, err := os.ReadFile(filepath.Join(root, fname))
		require.NoError(t, err)
		assert.Equal(t, strings.Join([]string{replacement, token, token}, " "), string(b))
	})

	t.Run("identity_replace_rewrites_unchanged_content", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("abc"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   "b",
			NewText:   "b",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, res.MatchCount)
		assert.Positive(t, res.BytesWritten)

		b, err := os.ReadFile(filepath.Join(root, fname))
		require.NoError(t, err)
		assert.Equal(t, "abc", string(b))
	})

	t.Run("old_text_not_found", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		body := fake.Lorem().Sentence(6)
		missing := fake.Lorem().Word() + fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(body), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   missing,
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "old_text not found")
	})

	t.Run("rejects_empty_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      "   ",
			OldText:   "a",
			NewText:   "b",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "path is required")
	})

	t.Run("rejects_empty_old_text", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fake.UUID().V4() + ".txt",
			OldText:   "",
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "old_text")
	})

	t.Run("rejects_absolute_path", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		bad := filepath.Join(abs, fake.UUID().V4()+".txt")
		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      bad,
			OldText:   "a",
			NewText:   "b",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "relative")
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		token := fake.Lorem().Word()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(token), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = svc.EditFile(ctx, EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   token,
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("no_workspaces_configured", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		svc := &Service{maxReadBytes: DefaultMaxReadBytes}
		_, err := svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fake.UUID().V4() + ".txt",
			OldText:   fake.Lorem().Word(),
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "no workspaces configured")
	})

	t.Run("multiple_workspaces_require_workspace_field", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		body := fake.Lorem().Word()
		repl := fake.Lorem().Word()
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(a, fname), []byte(body), 0o644))

		svc, err := NewService([]WorkspaceMount{
			{Identifier: "a", Description: "test", Path: filepath.Clean(absA)},
			{Identifier: "b", Description: "test", Path: filepath.Clean(absB)},
		})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{Path: fname, OldText: body, NewText: repl})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Path:      fname,
			OldText:   body,
			NewText:   repl,
			Workspace: "a",
		})
		require.NoError(t, err)
	})

	t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		body := fake.Lorem().Word()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(body), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Path:      fname,
			OldText:   body,
			NewText:   fake.Lorem().Word(),
			Workspace: "bogus",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "not configured")
	})

	t.Run("workspace_required_even_with_single_workspace", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		body := fake.Lorem().Word()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(body), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{Path: fname, OldText: body, NewText: fake.Lorem().Word()})
		require.Error(t, err)
		assert.ErrorContains(t, err, "workspace is required")
	})

	t.Run("invalid_utf8_in_old_text", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fake.UUID().V4() + ".txt",
			OldText:   string([]byte{0xff}),
			NewText:   "a",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "UTF-8")
	})

	t.Run("file_exceeds_max_read_bytes", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		runes := []rune(fake.Lorem().Word())
		require.NotEmpty(t, runes)
		ch := string(runes[0])
		repl := string(runes[len(runes)-1])
		if ch == repl {
			repl += "!"
		}
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		long := strings.Repeat(ch, 100)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(long), 0o644))

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxReadBytes(50),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   ch,
			NewText:   repl,
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "maximum read size")
	})

	t.Run("open_fails_permission_denied", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permission model differs on Windows")
		}
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		body := fake.Lorem().Word()
		require.NotEmpty(t, body)
		first := string([]rune(body)[0])
		last := strings.ToUpper(first)
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		p := filepath.Join(root, fname)
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		require.NoError(t, os.Chmod(p, 0o000))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   first,
			NewText:   last,
		})
		require.Error(t, err)
	})

	t.Run("read_fails_on_directory_path", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		dirName := fake.Lorem().Word()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.Mkdir(filepath.Join(root, dirName), 0o755))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      dirName,
			OldText:   fake.Lorem().Word(),
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
	})

	t.Run("write_fails_when_file_read_only", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permission model differs on Windows")
		}
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		p := filepath.Join(root, fname)
		require.NoError(t, os.WriteFile(p, []byte("ab"), 0o644))
		require.NoError(t, os.Chmod(p, 0o444))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		t.Cleanup(func() { _ = os.Chmod(p, 0o644) })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   "a",
			NewText:   "z",
		})
		require.Error(t, err)
	})

	t.Run("binary_file_rejected", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".dat"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte{0xff, 0xfe}, 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.EditFile(t.Context(), EditFileRequest{
			Workspace: "w",
			Path:      fname,
			OldText:   fake.Lorem().Word(),
			NewText:   fake.Lorem().Word(),
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "valid UTF-8")
	})
}
