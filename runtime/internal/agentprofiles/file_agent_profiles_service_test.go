package agentprofiles

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileAgentProfilesService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T, baseDir string) *FileAgentProfilesService {
		t.Helper()
		svc, err := NewFileAgentProfilesService(baseDir, testLogger(t))
		require.NoError(t, err)
		return svc
	}

	makeCreateParams := func() CreateAgentProfileParams {
		return CreateAgentProfileParams{
			Name:         fake.Lexify("profile-????????"),
			DisplayName:  fake.Person().Name(),
			Role:         "assistant",
			Instructions: fake.Lorem().Sentence(8),
			ToolRefs: []string{
				"tool.read",
				"tool.write",
			},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model",
			},
		}
	}

	t.Run("Create/Get/List/Delete", func(t *testing.T) {
		baseDir := t.TempDir()
		svc := makeService(t, baseDir)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		require.NotNil(t, created)

		filePath := filepath.Join(baseDir, "agent-profiles", created.Name+".yaml")
		_, err = os.Stat(filePath)
		require.NoError(t, err)

		got, err := svc.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, got.Name)
		assert.Equal(t, created.CreatedAt, got.CreatedAt)

		listed, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, created.Name, listed[0].Name)

		err = svc.Delete(ctx, created.Name)
		require.NoError(t, err)

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNotFound)
	})

	t.Run("Create returns conflict for duplicate name", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()
		params := makeCreateParams()

		_, err := svc.Create(ctx, params)
		require.NoError(t, err)

		_, err = svc.Create(ctx, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNameConflict)
	})

	t.Run("List returns profiles sorted by created_at", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		first, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		second, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)

		listed, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		assert.Equal(t, first.Name, listed[0].Name)
		assert.Equal(t, second.Name, listed[1].Name)
	})

	t.Run("Update changes mutable fields and preserves immutable fields", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)

		updated, err := svc.Update(ctx, created.Name, UpdateAgentProfileParams{
			DisplayName:  " Updated Name ",
			Role:         " reviewer ",
			Instructions: " updated instructions ",
			ToolRefs:     []string{" tool.write ", "tool.read", "tool.write"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: " provider/new-model ",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, created.Name, updated.Name)
		assert.Equal(t, created.CreatedAt, updated.CreatedAt)
		assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
		assert.Equal(t, "Updated Name", updated.DisplayName)
		assert.Equal(t, "reviewer", updated.Role)
		assert.Equal(t, "updated instructions", updated.Instructions)
		assert.Equal(t, []string{"tool.write", "tool.read"}, updated.ToolRefs)
		assert.Equal(t, "provider/new-model", updated.ExecutionSettings.DefaultModel)
	})

	t.Run("Update/Delete return not found for unknown profile", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		ctx := t.Context()

		_, err := svc.Update(ctx, "missing-profile", UpdateAgentProfileParams{
			DisplayName:  "x",
			Role:         "assistant",
			Instructions: "x",
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model",
			},
		})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAgentProfileNotFound)

		err = svc.Delete(ctx, "missing-profile")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrAgentProfileNotFound)
	})

	t.Run("AutoMigrate is no-op", func(t *testing.T) {
		svc := makeService(t, t.TempDir())
		require.NoError(t, svc.AutoMigrate())
	})

	t.Run("restart-shaped reload reads unchanged profile", func(t *testing.T) {
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
		t.Run("NewFileAgentProfilesService rejects empty base dir", func(t *testing.T) {
			_, err := NewFileAgentProfilesService("", testLogger(t))
			require.Error(t, err)
		})

		t.Run("List returns error on unreadable directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			profilesDir := filepath.Join(baseDir, "agent-profiles")
			require.NoError(t, os.Chmod(profilesDir, 0000))
			t.Cleanup(func() { _ = os.Chmod(profilesDir, 0750) })

			_, err := svc.List(t.Context())
			require.Error(t, err)
		})

		t.Run("List returns empty when profiles dir is missing", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			profilesDir := filepath.Join(baseDir, "agent-profiles")
			require.NoError(t, os.RemoveAll(profilesDir))

			listed, err := svc.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, listed)
		})

		t.Run("List returns error on corrupt yaml", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "agent-profiles", "bad.yaml")
			require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

			_, err := svc.List(t.Context())
			require.Error(t, err)
		})

		t.Run("Get returns parse error on corrupt yaml", func(t *testing.T) {
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			path := filepath.Join(baseDir, "agent-profiles", "bad.yaml")
			require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

			_, err := svc.Get(t.Context(), "bad")
			require.Error(t, err)
		})

		t.Run("Create returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			_, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         "invalid",
				Role:         "assistant",
				Instructions: "ok",
				ToolRefs:     []string{" "},
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
		})

		t.Run("Update returns validation error for invalid payload", func(t *testing.T) {
			svc := makeService(t, t.TempDir())
			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			_, err = svc.Update(t.Context(), created.Name, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: " ",
				},
			})
			require.Error(t, err)
		})

		t.Run("Delete returns remove error when path is a directory", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			baseDir := t.TempDir()
			svc := makeService(t, baseDir)
			ctx := t.Context()
			created, err := svc.Create(ctx, makeCreateParams())
			require.NoError(t, err)

			path := filepath.Join(baseDir, "agent-profiles", created.Name+".yaml")
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0750))
			require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600))
			t.Cleanup(func() { _ = os.RemoveAll(path) })

			err = svc.Delete(ctx, created.Name)
			require.Error(t, err)
		})
	})
}
