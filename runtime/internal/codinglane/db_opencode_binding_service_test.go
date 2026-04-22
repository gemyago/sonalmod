package codinglane

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseOpenCodeBindingService(t *testing.T) {
	fake := faker.New()

	testLogger := func() *slog.Logger {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	makeService := func(t *testing.T, dsn string, tablePrefix string) OpenCodeBindingService {
		t.Helper()
		svc, err := NewDatabaseOpenCodeBindingService(dsn, testLogger(), tablePrefix)
		require.NoError(t, err)
		require.NoError(t, svc.AutoMigrate())
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
		svc := makeService(t, ":memory:", "")
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
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
		assert.Equal(t, created.CreatedAt.UnixNano(), updated.CreatedAt.UnixNano())
		assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))

		err = svc.Delete(ctx, created.Name)
		require.NoError(t, err)

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOpenCodeBindingNotFound)
	})

	t.Run("Create returns conflict for duplicate name", func(t *testing.T) {
		svc := makeService(t, ":memory:", "")
		ctx := t.Context()
		params := makeCreateParams()

		_, err := svc.Create(ctx, params)
		require.NoError(t, err)

		_, err = svc.Create(ctx, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrOpenCodeBindingNameConflict)
	})

	t.Run("restart-shaped reload works with shared sqlite memory dsn", func(t *testing.T) {
		dsn := fmt.Sprintf("file:opencodebindings-%d?mode=memory&cache=shared", time.Now().UnixNano())
		ctx := t.Context()
		params := makeCreateParams()

		svc1 := makeService(t, dsn, "pref_")
		created, err := svc1.Create(ctx, params)
		require.NoError(t, err)

		svc2 := makeService(t, dsn, "pref_")
		loaded, err := svc2.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, loaded.Name)
		assert.Equal(t, created.ProfileName, loaded.ProfileName)
		assert.Equal(t, created.CWD, loaded.CWD)
		assert.Equal(t, created.AgentCommand, loaded.AgentCommand)
		assert.Equal(t, created.LaunchOptions, loaded.LaunchOptions)
		assert.Equal(t, created.CreatedAt.UnixNano(), loaded.CreatedAt.UnixNano())
		assert.Equal(t, created.UpdatedAt.UnixNano(), loaded.UpdatedAt.UnixNano())
	})

	t.Run("constructor and error paths", func(t *testing.T) {
		t.Run("creates service with sqlite memory dsn", func(t *testing.T) {
			svc, err := NewDatabaseOpenCodeBindingService(":memory:", nil, "")
			require.NoError(t, err)
			require.NotNil(t, svc)
		})

		t.Run("fails with invalid postgres dsn", func(t *testing.T) {
			svc, err := NewDatabaseOpenCodeBindingService(
				"postgres://localhost:1/db",
				nil,
				"",
			)
			require.Error(t, err)
			assert.Nil(t, svc)
		})

		t.Run("AutoMigrate is idempotent", func(t *testing.T) {
			svc, err := NewDatabaseOpenCodeBindingService(":memory:", nil, "")
			require.NoError(t, err)
			require.NoError(t, svc.AutoMigrate())
			require.NoError(t, svc.AutoMigrate())
		})

		t.Run("Update and Delete return not found for unknown binding", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
			_, err := svc.Update(t.Context(), "missing-binding", UpdateOpenCodeBindingParams{
				AgentCommand: OpenCodeAgentCommand{
					Command: "opencode",
					Args:    []string{"run"},
				},
				LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOpenCodeBindingNotFound)

			err = svc.Delete(t.Context(), "missing-binding")
			require.Error(t, err)
			require.ErrorIs(t, err, ErrOpenCodeBindingNotFound)
		})

		t.Run("Create and Update return validation errors", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
			_, err := svc.Create(t.Context(), CreateOpenCodeBindingParams{
				Name:        "binding-ok",
				ProfileName: "profile-ok",
			})
			require.Error(t, err)

			created, err := svc.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			_, err = svc.Update(t.Context(), created.Name, UpdateOpenCodeBindingParams{
				AgentCommand: OpenCodeAgentCommand{
					Command: " ",
				},
			})
			require.Error(t, err)
		})

		t.Run("closed db returns operation errors", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
			concrete := svc.(*DatabaseOpenCodeBindingService)
			sqlDB, err := concrete.db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())

			_, err = svc.List(t.Context())
			require.Error(t, err)
			_, err = svc.Get(t.Context(), "any")
			require.Error(t, err)
			_, err = svc.Create(t.Context(), makeCreateParams())
			require.Error(t, err)
			_, err = svc.Update(t.Context(), "any", UpdateOpenCodeBindingParams{
				AgentCommand: OpenCodeAgentCommand{
					Command: "opencode",
					Args:    []string{"run"},
				},
				LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
			})
			require.Error(t, err)
			err = svc.Delete(t.Context(), "any")
			require.Error(t, err)
		})
	})
}
