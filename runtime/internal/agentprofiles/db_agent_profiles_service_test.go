package agentprofiles

import (
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
		assert.ErrorIs(t, err, ErrAgentProfileNotFound)

		err = svc.Delete(ctx, "missing-profile")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAgentProfileNotFound)
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
}
