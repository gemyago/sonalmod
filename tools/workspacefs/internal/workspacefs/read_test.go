package workspacefs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTextFile(t *testing.T) {
	t.Parallel()

	one := func(path string) []WorkspaceMount {
		return []WorkspaceMount{{Identifier: "w", Description: "test", Path: path}}
	}
	two := func(p1, p2 string) []WorkspaceMount {
		return []WorkspaceMount{
			{Identifier: "a", Description: "test", Path: p1},
			{Identifier: "b", Description: "test", Path: p2},
		}
	}

	t.Run("request_JSON_round_trip_uses_workspace", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		p := fake.UUID().V4() + ".txt"
		in := ReadTextFileRequest{Path: p, Workspace: "codebase"}
		b, err := json.Marshal(in)
		require.NoError(t, err)
		assert.Contains(t, string(b), `"workspace":"codebase"`)
		assert.NotContains(t, string(b), `"root"`)
		var out ReadTextFileRequest
		require.NoError(t, json.Unmarshal(b, &out))
		assert.Equal(t, in, out)
	})

	t.Run("reads_file_inside_root", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("hello world"), 0o644))

		svc, err := NewService(one(clean))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname})
		require.NoError(t, err)
		assert.Equal(t, ReadTextFileResponse{Content: "hello world"}, res)
	})

	t.Run("rejects_absolute_path", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		x := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		bad := filepath.Join(abs, x)
		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: bad})
		require.Error(t, err)
		assert.ErrorContains(t, err, "relative")
	})

	t.Run("rejects_path_outside_root_via_dotdot", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "../../etc/passwd"})
		require.Error(t, err)
	})

	t.Run("head_returns_first_n_lines", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		content := "line1\nline2\nline3\nline4"
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(content), 0o644))

		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		n := 2
		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Head: &n})
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2", res.Content)
	})

	t.Run("tail_returns_last_n_lines", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		content := "a\nb\nc\nd"
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(content), 0o644))

		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		n := 2
		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Tail: &n})
		require.NoError(t, err)
		assert.Equal(t, "c\nd", res.Content)
	})

	t.Run("empty_file", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), nil, 0o644))

		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname})
		require.NoError(t, err)
		assert.Equal(t, ReadTextFileResponse{Content: ""}, res)
	})

	t.Run("head_and_tail_mutually_exclusive", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		h, tail := 1, 1
		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "x", Head: &h, Tail: &tail})
		require.Error(t, err)
		assert.ErrorContains(t, err, "mutually exclusive")
	})

	t.Run("rejects_file_larger_than_max_bytes", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		big := strings.Repeat("x", 128)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte(big), 0o644))

		svc, err := NewService(one(filepath.Clean(abs)), WithMaxReadBytes(64))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname})
		require.Error(t, err)
		assert.ErrorContains(t, err, "maximum read size")
	})

	t.Run("invalid_utf8_rejected", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte{0xff, 0xfe, 0xfd}, 0o644))

		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname})
		require.Error(t, err)
		assert.ErrorContains(t, err, "UTF-8")
	})

	t.Run("multiple_workspaces_require_workspace_field", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(a, fname), []byte("A"), 0o644))

		svc, err := NewService(two(filepath.Clean(absA), filepath.Clean(absB)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: fname})
		require.Error(t, err)
		require.ErrorContains(t, err, "workspace is required")

		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: fname, Workspace: "a"})
		require.NoError(t, err)
		assert.Equal(t, "A", res.Content)
	})

	t.Run("cancelled_context", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		missing := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = svc.ReadTextFile(ctx, ReadTextFileRequest{Workspace: "w", Path: missing})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("no_workspaces_configured", func(t *testing.T) {
		t.Parallel()
		svc := &Service{maxReadBytes: DefaultMaxReadBytes}
		_, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "a"})
		require.Error(t, err)
		require.ErrorContains(t, err, "no workspaces configured")
	})

	t.Run("negative_head_and_tail", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		neg := -1
		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "x", Head: &neg})
		require.Error(t, err)
		require.ErrorContains(t, err, "non-negative")

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "x", Tail: &neg})
		require.Error(t, err)
		require.ErrorContains(t, err, "non-negative")
	})

	t.Run("empty_path", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: "  "})
		require.Error(t, err)
		require.ErrorContains(t, err, "path is required")
	})

	t.Run("file_not_found", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		missing := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: missing})
		require.Error(t, err)
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("unknown_workspace_single_mount", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: "x", Workspace: "bogus"})
		require.Error(t, err)
		require.ErrorContains(t, err, "not configured")
	})

	t.Run("multiple_workspaces_unknown_workspace", func(t *testing.T) {
		t.Parallel()
		a := t.TempDir()
		b := t.TempDir()
		absA, err := filepath.Abs(a)
		require.NoError(t, err)
		absB, err := filepath.Abs(b)
		require.NoError(t, err)

		svc, err := NewService(two(filepath.Clean(absA), filepath.Clean(absB)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Path: "x", Workspace: "not-a-workspace"})
		require.Error(t, err)
		require.ErrorContains(t, err, "not configured")
	})

	t.Run("head_zero_and_tail_zero", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("a\nb\nc"), 0o644))
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		z := 0
		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Head: &z})
		require.NoError(t, err)
		assert.Empty(t, res.Content)

		res, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Tail: &z})
		require.NoError(t, err)
		assert.Empty(t, res.Content)
	})

	t.Run("head_tail_fewer_lines_than_limit", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fname := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("only"), 0o644))
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		n := 10
		res, err := svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Head: &n})
		require.NoError(t, err)
		assert.Equal(t, "only", res.Content)

		res, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: fname, Tail: &n})
		require.NoError(t, err)
		assert.Equal(t, "only", res.Content)
	})

	t.Run("read_directory_returns_error", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		dirName := fake.UUID().V4()
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(root, dirName), 0o755))
		svc, err := NewService(one(filepath.Clean(abs)))
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		_, err = svc.ReadTextFile(t.Context(), ReadTextFileRequest{Workspace: "w", Path: dirName})
		require.Error(t, err)
	})
}

func TestReadAllLimited(t *testing.T) {
	t.Parallel()

	t.Run("non_positive_limit", func(t *testing.T) {
		t.Parallel()
		_, err := readAllLimited(strings.NewReader("a"), 0)
		require.Error(t, err)
		require.ErrorContains(t, err, "positive")
	})

	t.Run("read_error_propagates", func(t *testing.T) {
		t.Parallel()
		_, err := readAllLimited(errReader{}, 10)
		require.Error(t, err)
		require.ErrorContains(t, err, "read file")
	})
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

var _ io.Reader = errReader{}

func TestReadMultipleFiles(t *testing.T) {
	t.Parallel()

	t.Run("partial_success_one_missing", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		fa := fake.UUID().V4() + ".txt"
		fb := fake.UUID().V4() + ".txt"
		missing := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, fa), []byte("alpha"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, fb), []byte("beta"), 0o644))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{
			Paths:     []string{fa, missing, fb},
			Workspace: "w",
		})
		require.NoError(t, err)
		require.Len(t, res.Results, 3)

		assert.Equal(t, fa, res.Results[0].Path)
		assert.Equal(t, "alpha", res.Results[0].Content)
		assert.Empty(t, res.Results[0].Error)

		assert.Equal(t, missing, res.Results[1].Path)
		assert.Empty(t, res.Results[1].Content)
		require.NotEmpty(t, res.Results[1].Error)

		assert.Equal(t, fb, res.Results[2].Path)
		assert.Equal(t, "beta", res.Results[2].Content)
		assert.Empty(t, res.Results[2].Error)
	})

	t.Run("handler_errors", func(t *testing.T) {
		t.Run("cancelled_before_start", func(t *testing.T) {
			t.Parallel()
			fake := faker.New()
			p := fake.UUID().V4() + ".txt"
			root := t.TempDir()
			abs, err := filepath.Abs(root)
			require.NoError(t, err)
			svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
			require.NoError(t, err)
			t.Cleanup(func() { _ = svc.Close() })

			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			_, err = svc.ReadMultipleFiles(ctx, ReadMultipleFilesRequest{Paths: []string{p}, Workspace: "w"})
			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
		})

		t.Run("pick_workspace_error_multiple_workspaces", func(t *testing.T) {
			t.Parallel()
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

			_, err = svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{Paths: []string{"x"}})
			require.Error(t, err)
			require.ErrorContains(t, err, "workspace is required")
		})

		t.Run("pick_workspace_unknown_identifier", func(t *testing.T) {
			t.Parallel()
			fake := faker.New()
			fname := fake.UUID().V4() + ".txt"
			root := t.TempDir()
			abs, err := filepath.Abs(root)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("a"), 0o644))
			svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
			require.NoError(t, err)
			t.Cleanup(func() { _ = svc.Close() })

			_, err = svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{
				Paths:     []string{fname},
				Workspace: "not-there",
			})
			require.Error(t, err)
			assert.ErrorContains(t, err, "not configured")
		})

		t.Run("workspace_required_even_with_single_workspace", func(t *testing.T) {
			t.Parallel()
			fake := faker.New()
			fname := fake.UUID().V4() + ".txt"
			root := t.TempDir()
			abs, err := filepath.Abs(root)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, fname), []byte("a"), 0o644))
			svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: filepath.Clean(abs)}})
			require.NoError(t, err)
			t.Cleanup(func() { _ = svc.Close() })

			_, err = svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{Paths: []string{fname}})
			require.Error(t, err)
			assert.ErrorContains(t, err, "workspace is required")
		})
	})

	t.Run("partial_path_failures", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		ok := fake.UUID().V4() + ".txt"
		big := fake.UUID().V4() + ".txt"
		bad := fake.UUID().V4() + ".txt"
		sub := fake.UUID().V4()
		nosuch := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		require.NoError(t, os.WriteFile(filepath.Join(root, ok), []byte("ok"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, big), []byte(strings.Repeat("x", 128)), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, bad), []byte{0xff, 0xfe}, 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, sub), 0o755))

		svc, err := NewService(
			[]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}},
			WithMaxReadBytes(64),
		)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{
			Workspace: "w",
			Paths: []string{
				"",
				"../../etc/passwd",
				ok,
				nosuch,
				bad,
				big,
				sub,
			},
		})
		require.NoError(t, err)
		require.Len(t, res.Results, 7)

		assert.Contains(t, res.Results[0].Error, "path is required")
		assert.Contains(t, res.Results[1].Error, "escapes")
		assert.Equal(t, "ok", res.Results[2].Content)
		require.NotEmpty(t, res.Results[3].Error)
		assert.Contains(t, res.Results[4].Error, "UTF-8")
		assert.Contains(t, res.Results[5].Error, "maximum read size")
		require.NotEmpty(t, res.Results[6].Error)
	})

	t.Run("cancel_between_paths", func(t *testing.T) {
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

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		ctx := &errOnNthCall{base: t.Context(), at: 3}
		_, err = svc.ReadMultipleFiles(ctx, ReadMultipleFilesRequest{Paths: []string{fa, fb}, Workspace: "w"})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("open_error_not_notexist", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		sec := fake.UUID().V4() + ".txt"
		root := t.TempDir()
		abs, err := filepath.Abs(root)
		require.NoError(t, err)
		clean := filepath.Clean(abs)
		p := filepath.Join(root, sec)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o000))

		svc, err := NewService([]WorkspaceMount{{Identifier: "w", Description: "test", Path: clean}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })

		res, err := svc.ReadMultipleFiles(t.Context(), ReadMultipleFilesRequest{
			Paths:     []string{sec},
			Workspace: "w",
		})
		require.NoError(t, err)
		require.Len(t, res.Results, 1)
		require.NotEmpty(t, res.Results[0].Error)
		assert.NotContains(t, res.Results[0].Error, "file does not exist")
	})
}

// errOnNthCall implements [context.Context] and returns [context.Canceled] from Err()
// starting at the n-th invocation (1-based), so the batch read loop can be interrupted
// between paths.
type errOnNthCall struct {
	base context.Context
	n    int
	at   int
}

func (e *errOnNthCall) Deadline() (time.Time, bool) {
	return e.base.Deadline()
}

func (e *errOnNthCall) Done() <-chan struct{} {
	return e.base.Done()
}

func (e *errOnNthCall) Err() error {
	e.n++
	if e.n >= e.at {
		return context.Canceled
	}
	return e.base.Err()
}

func (e *errOnNthCall) Value(key any) any {
	return e.base.Value(key)
}
