package agentprofiles

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseAgentProfilesService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T, dsn string, tablePrefix string) *DatabaseAgentProfilesService {
		t.Helper()
		svc, err := NewDatabaseAgentProfilesService(dsn, testLogger(t), tablePrefix)
		require.NoError(t, err)
		require.NoError(t, svc.AutoMigrate())
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

	readStoredExecutionSettings := func(
		t *testing.T,
		svc *DatabaseAgentProfilesService,
		name string,
	) map[string]any {
		t.Helper()

		var raw string
		require.NoError(
			t,
			svc.db.Raw(
				"SELECT execution_settings FROM agent_profiles WHERE name = ?",
				name,
			).Scan(&raw).Error,
		)
		require.NotEmpty(t, raw)

		var payload map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &payload))
		return payload
	}

	t.Run("NewDatabaseAgentProfilesService", func(t *testing.T) {
		t.Run("creates service with sqlite memory dsn", func(t *testing.T) {
			svc, err := NewDatabaseAgentProfilesService(":memory:", nil, "")
			require.NoError(t, err)
			require.NotNil(t, svc)
		})

		t.Run("fails with invalid postgres dsn", func(t *testing.T) {
			svc, err := NewDatabaseAgentProfilesService(
				"postgres://localhost:"+strconv.Itoa(fake.RandomNumber(10000))+"/db",
				nil,
				"",
			)
			require.Error(t, err)
			assert.Nil(t, svc)
		})
	})

	t.Run("AutoMigrate is idempotent", func(t *testing.T) {
		svc, err := NewDatabaseAgentProfilesService(":memory:", nil, "")
		require.NoError(t, err)
		require.NoError(t, svc.AutoMigrate())
		require.NoError(t, svc.AutoMigrate())
	})

	t.Run("Create/Get/List/Delete", func(t *testing.T) {
		svc := makeService(t, ":memory:", "")
		ctx := t.Context()

		created, err := svc.Create(ctx, makeCreateParams())
		require.NoError(t, err)
		require.NotNil(t, created)

		got, err := svc.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, got.Name)
		assert.True(t, got.CreatedAt.Equal(created.CreatedAt))

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
		svc := makeService(t, ":memory:", "")
		ctx := t.Context()
		params := makeCreateParams()

		_, err := svc.Create(ctx, params)
		require.NoError(t, err)

		_, err = svc.Create(ctx, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNameConflict)
	})

	t.Run("List returns profiles sorted by created_at", func(t *testing.T) {
		svc := makeService(t, ":memory:", "")
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
		svc := makeService(t, ":memory:", "")
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
		assert.True(t, updated.CreatedAt.Equal(created.CreatedAt))
		assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))
		assert.Equal(t, "Updated Name", updated.DisplayName)
		assert.Equal(t, "reviewer", updated.Role)
		assert.Equal(t, "updated instructions", updated.Instructions)
		assert.Equal(t, []string{"tool.write", "tool.read"}, updated.ToolRefs)
		assert.Equal(t, "provider/new-model", updated.ExecutionSettings.DefaultModel)
	})

	t.Run("Update/Delete return not found for unknown profile", func(t *testing.T) {
		svc := makeService(t, ":memory:", "")
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

	t.Run("restart-shaped reload works with shared sqlite memory dsn", func(t *testing.T) {
		dsn := fmt.Sprintf("file:agentprofiles-%d?mode=memory&cache=shared", time.Now().UnixNano())
		ctx := t.Context()
		params := makeCreateParams()

		svc1 := makeService(t, dsn, "prefix_")
		created, err := svc1.Create(ctx, params)
		require.NoError(t, err)

		svc2 := makeService(t, dsn, "prefix_")
		loaded, err := svc2.Get(ctx, created.Name)
		require.NoError(t, err)
		assert.Equal(t, created.Name, loaded.Name)
		assert.Equal(t, created.DisplayName, loaded.DisplayName)
		assert.Equal(t, created.Role, loaded.Role)
		assert.Equal(t, created.Instructions, loaded.Instructions)
		assert.Equal(t, created.ToolRefs, loaded.ToolRefs)
		assert.Equal(t, created.ExecutionSettings, loaded.ExecutionSettings)
		assert.Equal(t, created.CreatedAt.UnixNano(), loaded.CreatedAt.UnixNano())
		assert.Equal(t, created.UpdatedAt.UnixNano(), loaded.UpdatedAt.UnixNano())
	})

	t.Run("round-trips execution settings variants", func(t *testing.T) {
		t.Run("explicit regular mode persists execution settings mode", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")

			created, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         fake.Lexify("profile-????????"),
				DisplayName:  fake.Person().Name(),
				Role:         "assistant",
				Instructions: fake.Lorem().Sentence(8),
				ToolRefs:     []string{"tool.read"},
				ExecutionSettings: ExecutionSettings{
					Mode:         ExecutionModeRegular,
					DefaultModel: "provider/model",
				},
			})
			require.NoError(t, err)

			stored := readStoredExecutionSettings(t, svc, created.Name)
			assert.Equal(t, string(ExecutionModeRegular), stored["mode"])
			assert.Equal(t, "provider/model", stored["defaultModel"])
			assert.NotContains(t, stored, "agentCommand")
			assert.NotContains(t, stored, "cwd")

			reloaded, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.ExecutionSettings, reloaded.ExecutionSettings)
		})

		t.Run("acp-stdio mode persists command settings", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")

			created, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         fake.Lexify("profile-????????"),
				DisplayName:  fake.Person().Name(),
				Role:         "assistant",
				Instructions: fake.Lorem().Sentence(8),
				ToolRefs:     []string{"tool.read"},
				ExecutionSettings: ExecutionSettings{
					Mode: ExecutionModeACPStdio,
					AgentCommand: ACPStdioAgentCommand{
						Command: "opencode",
						Args:    []string{"acp", "--safe"},
					},
					Cwd: "/workspace",
				},
			})
			require.NoError(t, err)

			stored := readStoredExecutionSettings(t, svc, created.Name)
			assert.Equal(t, string(ExecutionModeACPStdio), stored["mode"])
			assert.Equal(t, "/workspace", stored["cwd"])
			assert.NotContains(t, stored, "defaultModel")
			require.Contains(t, stored, "agentCommand")
			assert.Equal(t, map[string]any{
				"command": "opencode",
				"args":    []any{"acp", "--safe"},
			}, stored["agentCommand"])

			reloaded, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.ExecutionSettings, reloaded.ExecutionSettings)
		})
	})

	t.Run("AutoMigrate preserves existing regular records", func(t *testing.T) {
		svc, err := NewDatabaseAgentProfilesService(":memory:", testLogger(t), "")
		require.NoError(t, err)

		require.NoError(t, svc.db.Exec(`
			CREATE TABLE agent_profiles (
				name TEXT PRIMARY KEY,
				display_name TEXT,
				role TEXT NOT NULL,
				instructions TEXT NOT NULL,
				tool_refs TEXT,
				execution_settings TEXT,
				created_at DATETIME,
				updated_at DATETIME
			)
		`).Error)

		name := fake.Lexify("profile-????????")
		createdAt := time.Now().UTC().Add(-time.Minute).Round(0)
		updatedAt := createdAt.Add(2 * time.Second)

		toolRefsJSON, err := json.Marshal([]string{"tool.read"})
		require.NoError(t, err)
		execSettingsJSON, err := json.Marshal(map[string]any{
			"defaultModel": "provider/model",
		})
		require.NoError(t, err)

		require.NoError(
			t,
			svc.db.Exec(
				`INSERT INTO agent_profiles
					(name, display_name, role, instructions, tool_refs, execution_settings, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				name,
				fake.Person().Name(),
				"assistant",
				fake.Lorem().Sentence(8),
				string(toolRefsJSON),
				string(execSettingsJSON),
				createdAt,
				updatedAt,
			).Error,
		)

		require.NoError(t, svc.AutoMigrate())

		profile, err := svc.Get(t.Context(), name)
		require.NoError(t, err)
		assert.Equal(t, ExecutionSettings{
			DefaultModel: "provider/model",
		}, profile.ExecutionSettings)
		assert.Equal(t, createdAt.UnixNano(), profile.CreatedAt.UnixNano())
		assert.Equal(t, updatedAt.UnixNano(), profile.UpdatedAt.UnixNano())
	})

	t.Run("validation and database error paths", func(t *testing.T) {
		t.Run("Create returns validation errors", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
			_, err := svc.Create(t.Context(), CreateAgentProfileParams{
				Name:         "profile-1",
				Role:         " ",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
		})

		t.Run("Update returns validation errors", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
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

		t.Run("closed db returns operation errors", func(t *testing.T) {
			svc := makeService(t, ":memory:", "")
			sqlDB, err := svc.db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())

			_, err = svc.List(t.Context())
			require.Error(t, err)

			_, err = svc.Get(t.Context(), "any")
			require.Error(t, err)

			_, err = svc.Create(t.Context(), makeCreateParams())
			require.Error(t, err)

			_, err = svc.Update(t.Context(), "any", UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)

			err = svc.Delete(t.Context(), "any")
			require.Error(t, err)
		})
	})
}
