package codinglane

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
}
