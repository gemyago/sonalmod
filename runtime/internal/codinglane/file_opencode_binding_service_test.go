package codinglane

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileOpenCodeBindingService(t *testing.T) {
	fake := faker.New()

	testLogger := func() *slog.Logger {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	makeService := func(t *testing.T, baseDir string) OpenCodeBindingService {
		t.Helper()
		svc, err := NewFileOpenCodeBindingService(baseDir, testLogger())
		require.NoError(t, err)
		return svc
	}

	makeCreateParams := func() CreateOpenCodeBindingParams {
		return CreateOpenCodeBindingParams{
			Name:        fake.Lexify("binding-????????"),
			ProfileName: fake.Lexify("profile-????"),
			CWD:         filepath.Join("/tmp", fake.Lexify("work-????")),
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"run"},
			},
			LaunchOptions: OpenCodeLaunchOptions{
				Transport: "stdio",
			},
		}
	}

	t.Run("Create/Get/List/Update/Delete", func(t *testing.T) {
		baseDir := t.TempDir()
		ctx := t.Context()
		svc := makeService(t, baseDir)

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)

		bindingPath := filepath.Join(baseDir, "opencode-bindings", created.Name+".yaml")
		_, err = os.Stat(bindingPath)
		require.NoError(t, err)

		got, err := svc.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, got.Name)
		assert.Equal(t, created.ProfileName, got.ProfileName)

		listed, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, created.Name, listed[0].Name)

		time.Sleep(2 * time.Millisecond)
		updated, err := svc.Update(ctx, created.Name, UpdateOpenCodeBindingParams{
			CWD: filepath.Join("/tmp", "updated"),
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"run", "--json"},
			},
			LaunchOptions: OpenCodeLaunchOptions{
				Transport: "stdio",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, created.Name, updated.Name)
		assert.Equal(t, created.ProfileName, updated.ProfileName)
		assert.Equal(t, created.CreatedAt, updated.CreatedAt)
		assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))

		err = svc.Delete(ctx, created.Name)
		require.NoError(t, err)

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOpenCodeBindingNotFound)
	})

	t.Run("Create returns conflict for duplicate name", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()
		params := makeCreateParams()

		_, err := svc.Create(ctx, params)
		require.NoError(t, err)

		_, err = svc.Create(ctx, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOpenCodeBindingNameConflict)
	})

	t.Run("restart-shaped reload reads unchanged binding", func(t *testing.T) {
		baseDir := t.TempDir()
		ctx := t.Context()
		params := makeCreateParams()

		svc1 := makeService(t, baseDir)
		created, err := svc1.Create(ctx, params)
		require.NoError(t, err)

		svc2 := makeService(t, baseDir)
		loaded, err := svc2.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, *created, *loaded)
	})

	t.Run("error paths", func(t *testing.T) {
		t.Run("NewFileOpenCodeBindingService rejects empty base dir", func(t *testing.T) {
			_, err := NewFileOpenCodeBindingService("", testLogger())
			require.Error(t, err)
		})

		t.Run("NewFileOpenCodeBindingService returns mkdir error when base path is file", func(t *testing.T) {
			base := t.TempDir()
			filePath := filepath.Join(base, "not-dir")
			require.NoError(t, os.WriteFile(filePath, []byte("x"), 0600))

			_, err := NewFileOpenCodeBindingService(filePath, testLogger())
			require.Error(t, err)
		})

		t.Run("List returns empty when bindings dir is missing", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			require.NoError(t, os.RemoveAll(filepath.Join(baseDir, "opencode-bindings")))

			listed, err := svc.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, listed)
		})

		t.Run("List returns error on unreadable directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			dir := filepath.Join(baseDir, "opencode-bindings")
			require.NoError(t, os.Chmod(dir, 0000))
			t.Cleanup(func() { _ = os.Chmod(dir, 0750) })

			_, err := svc.List(t.Context())
			require.Error(t, err)
		})

		t.Run("List ignores non-yaml files and directories", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			bindingsDir := filepath.Join(baseDir, "opencode-bindings")
			require.NoError(t, os.WriteFile(filepath.Join(bindingsDir, "notes.txt"), []byte("x"), 0600))
			require.NoError(t, os.Mkdir(filepath.Join(bindingsDir, "subdir"), 0750))

			listed, err := svc.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, listed)
		})

		t.Run("List returns parse error on corrupt yaml", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "opencode-bindings", "bad.yaml")
			require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

			_, err := svc.List(t.Context())
			require.Error(t, err)
		})

		t.Run("Get returns parse error on corrupt yaml", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "opencode-bindings", "bad.yaml")
			require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

			_, err := svc.Get(t.Context(), "bad")
			require.Error(t, err)
		})

		t.Run("Create returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			_, err := svc.Create(t.Context(), CreateOpenCodeBindingParams{
				Name:        "binding-ok",
				ProfileName: "profile-ok",
				AgentCommand: OpenCodeAgentCommand{
					Command: " ",
				},
			})
			require.Error(t, err)
		})

		t.Run("Create returns write error when bindings dir is not writable", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			bindingsDir := filepath.Join(baseDir, "opencode-bindings")
			require.NoError(t, os.Chmod(bindingsDir, 0500))
			t.Cleanup(func() { _ = os.Chmod(bindingsDir, 0750) })

			_, err := svc.Create(t.Context(), makeCreateParams())
			require.Error(t, err)
		})

		t.Run("Update returns not found for unknown binding", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			_, err := svc.Update(t.Context(), "missing-binding", UpdateOpenCodeBindingParams{
				AgentCommand: OpenCodeAgentCommand{
					Command: "opencode",
					Args:    []string{"run"},
				},
				LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOpenCodeBindingNotFound)
		})

		t.Run("Update returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			_, err = svc.Update(t.Context(), created.Name, UpdateOpenCodeBindingParams{
				AgentCommand: OpenCodeAgentCommand{
					Command: " ",
				},
			})
			require.Error(t, err)
		})

		t.Run("Update returns write error when file is read-only", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)
			path := filepath.Join(baseDir, "opencode-bindings", created.Name+".yaml")
			require.NoError(t, os.Chmod(path, 0400))
			t.Cleanup(func() { _ = os.Chmod(path, 0600) })

			_, err = svc.Update(t.Context(), created.Name, UpdateOpenCodeBindingParams{
				CWD: "/tmp/updated",
				AgentCommand: OpenCodeAgentCommand{
					Command: "opencode",
					Args:    []string{"run"},
				},
				LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
			})
			require.Error(t, err)
		})

		t.Run("Delete returns remove error when path is directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			path := filepath.Join(baseDir, "opencode-bindings", created.Name+".yaml")
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0750))
			require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600))
			t.Cleanup(func() { _ = os.RemoveAll(path) })

			err = svc.Delete(t.Context(), created.Name)
			require.Error(t, err)
		})

		t.Run("Delete returns not found for unknown binding", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			err := svc.Delete(t.Context(), "missing-binding")
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOpenCodeBindingNotFound)
		})

		t.Run("Get returns read error when binding path is a directory", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "opencode-bindings", "dir-binding.yaml")
			require.NoError(t, os.Mkdir(path, 0750))

			_, err := svc.Get(t.Context(), "dir-binding")
			require.Error(t, err)
		})

		t.Run("AutoMigrate is no-op", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			require.NoError(t, svc.AutoMigrate())
		})
	})
}
