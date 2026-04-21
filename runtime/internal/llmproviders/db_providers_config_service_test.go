package llmproviders

import (
	"strconv"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseProvidersConfigService(t *testing.T) {
	fake := faker.New()
	randomProviderName := func() string {
		return fake.Lexify("provider-????????")
	}

	makeService := func(t *testing.T) *DatabaseProvidersConfigService {
		t.Helper()
		svc, err := NewDatabaseProvidersConfigService(
			":memory:",
			testLogger(t),
			"",
		)
		require.NoError(t, err)
		require.NotNil(t, svc)
		require.NoError(t, svc.AutoMigrate())
		return svc
	}

	makeParams := func() CreateProviderConfigParams {
		return CreateProviderConfigParams{
			Name:        randomProviderName(),
			Type:        ProviderTypeOpenAICompatible,
			DisplayName: fake.Company().Name(),
			BaseURL:     "https://" + fake.Internet().Domain(),
			APIKey:      "sk-" + fake.Lorem().Text(20),
		}
	}

	t.Run("NewDatabaseProvidersConfigService", func(t *testing.T) {
		t.Run("creates service with SQLite in-memory DSN", func(t *testing.T) {
			svc, err := NewDatabaseProvidersConfigService(":memory:", nil, "")
			require.NoError(t, err)
			require.NotNil(t, svc)
		})

		t.Run("fails with invalid postgres DSN", func(t *testing.T) {
			svc, err := NewDatabaseProvidersConfigService(
				"postgres://localhost:"+strconv.Itoa(fake.RandomNumber(10000))+"/"+fake.Lorem().Word(),
				nil,
				"",
			)
			require.Error(t, err)
			assert.Nil(t, svc)
		})
	})

	t.Run("AutoMigrate", func(t *testing.T) {
		t.Run("creates provider_configs table successfully", func(t *testing.T) {
			svc, err := NewDatabaseProvidersConfigService(":memory:", nil, "")
			require.NoError(t, err)
			err = svc.AutoMigrate()
			require.NoError(t, err)
		})

		t.Run("is idempotent - can be called multiple times", func(t *testing.T) {
			svc, err := NewDatabaseProvidersConfigService(":memory:", nil, "")
			require.NoError(t, err)
			require.NoError(t, svc.AutoMigrate())
			require.NoError(t, svc.AutoMigrate())
		})
	})

	insertModel := func(t *testing.T, svc *DatabaseProvidersConfigService) providerConfigModel {
		t.Helper()
		m := providerConfigModel{
			Name:        randomProviderName(),
			Type:        ProviderTypeOpenAICompatible,
			DisplayName: fake.Company().Name(),
			BaseURL:     "https://" + fake.Internet().Domain(),
			APIKey:      "sk-" + fake.Lorem().Text(20),
		}
		require.NoError(t, svc.db.Create(&m).Error)
		return m
	}

	t.Run("List", func(t *testing.T) {
		t.Run("returns empty slice when no providers exist", func(t *testing.T) {
			svc := makeService(t)
			result, err := svc.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, result)
		})

		t.Run("returns all providers sorted by created_at ascending", func(t *testing.T) {
			svc := makeService(t)
			m1 := insertModel(t, svc)
			m2 := insertModel(t, svc)

			result, err := svc.List(t.Context())
			require.NoError(t, err)
			require.Len(t, result, 2)
			assert.Equal(t, m1.Name, result[0].Name)
			assert.Equal(t, m2.Name, result[1].Name)
			assert.Equal(t, m1.Type, result[0].Type)
			assert.Equal(t, m1.DisplayName, result[0].DisplayName)
			assert.Equal(t, m1.BaseURL, result[0].BaseURL)
			assert.Equal(t, m1.APIKey, result[0].APIKey)
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("returns provider by name", func(t *testing.T) {
			svc := makeService(t)
			m := insertModel(t, svc)

			got, err := svc.Get(t.Context(), m.Name)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, m.Name, got.Name)
			assert.Equal(t, m.Type, got.Type)
			assert.Equal(t, m.DisplayName, got.DisplayName)
			assert.Equal(t, m.BaseURL, got.BaseURL)
			assert.Equal(t, m.APIKey, got.APIKey)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)
			_, err := svc.Get(t.Context(), fake.Lexify("nonexistent-????????"))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
	})

	t.Run("Create", func(t *testing.T) {
		t.Run("creates provider and returns it with timestamps", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()

			before := time.Now().Truncate(time.Millisecond)
			result, err := svc.Create(t.Context(), params)
			after := time.Now().Add(time.Millisecond)

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, params.Name, result.Name)
			assert.Equal(t, params.Type, result.Type)
			assert.Equal(t, params.DisplayName, result.DisplayName)
			assert.Equal(t, params.BaseURL, result.BaseURL)
			assert.Equal(t, params.APIKey, result.APIKey)
			assert.False(t, result.CreatedAt.Before(before), "CreatedAt should not be before start")
			assert.False(t, result.CreatedAt.After(after), "CreatedAt should not be after end")
			assert.False(t, result.UpdatedAt.Before(before), "UpdatedAt should not be before start")
			assert.False(t, result.UpdatedAt.After(after), "UpdatedAt should not be after end")
		})

		t.Run("persists provider so Get returns it", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()

			created, err := svc.Create(t.Context(), params)
			require.NoError(t, err)

			got, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.Name, got.Name)
			assert.Equal(t, created.APIKey, got.APIKey)
		})

		t.Run("returns ErrProviderConfigNameConflict for duplicate name", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()

			_, err := svc.Create(t.Context(), params)
			require.NoError(t, err)

			_, err = svc.Create(t.Context(), params)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNameConflict)
		})

		t.Run("rejects invalid name pattern - starts with digit", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()
			params.Name = "1invalid"

			_, err := svc.Create(t.Context(), params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name")
		})

		t.Run("rejects invalid name pattern - uppercase", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()
			params.Name = "InvalidName"

			_, err := svc.Create(t.Context(), params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name")
		})

		t.Run("accepts valid name with hyphens and digits", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()
			params.Name = "my-provider-1"

			result, err := svc.Create(t.Context(), params)
			require.NoError(t, err)
			assert.Equal(t, "my-provider-1", result.Name)
		})

		t.Run("rejects unsupported provider type", func(t *testing.T) {
			svc := makeService(t)
			params := makeParams()
			params.Type = "unsupported-type-" + fake.Lorem().Word()

			_, err := svc.Create(t.Context(), params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "type")
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("updates provider fields", func(t *testing.T) {
			svc := makeService(t)

			created, err := svc.Create(t.Context(), makeParams())
			require.NoError(t, err)

			time.Sleep(2 * time.Millisecond)
			newDisplayName := fake.Company().Name()
			newBaseURL := "https://" + fake.Internet().Domain()
			newAPIKey := "sk-new-" + fake.Lorem().Text(20)

			updated, err := svc.Update(t.Context(), created.Name, UpdateProviderConfigParams{
				DisplayName: newDisplayName,
				BaseURL:     newBaseURL,
				APIKey:      newAPIKey,
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
			assert.Equal(t, created.Name, updated.Name)
			assert.Equal(t, created.Type, updated.Type)
			assert.Equal(t, newDisplayName, updated.DisplayName)
			assert.Equal(t, newBaseURL, updated.BaseURL)
			assert.Equal(t, newAPIKey, updated.APIKey)
			assert.True(t, updated.UpdatedAt.After(created.UpdatedAt), "UpdatedAt should advance after update")
		})

		t.Run("preserves API key when params APIKey is empty", func(t *testing.T) {
			svc := makeService(t)

			created, err := svc.Create(t.Context(), makeParams())
			require.NoError(t, err)
			originalKey := created.APIKey

			updated, err := svc.Update(t.Context(), created.Name, UpdateProviderConfigParams{
				BaseURL: "https://" + fake.Internet().Domain(),
				APIKey:  "",
			})
			require.NoError(t, err)
			assert.Equal(t, originalKey, updated.APIKey)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)

			_, err := svc.Update(t.Context(), fake.Lexify("nonexistent-????????"), UpdateProviderConfigParams{
				BaseURL: "https://" + fake.Internet().Domain(),
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("deletes provider", func(t *testing.T) {
			svc := makeService(t)

			created, err := svc.Create(t.Context(), makeParams())
			require.NoError(t, err)

			err = svc.Delete(t.Context(), created.Name)
			require.NoError(t, err)

			_, err = svc.Get(t.Context(), created.Name)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)

			err := svc.Delete(t.Context(), fake.Lexify("nonexistent-????????"))
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
	})

	closedDB := func(t *testing.T) *DatabaseProvidersConfigService {
		t.Helper()
		svc := makeService(t)
		sqlDB, err := svc.db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		return svc
	}

	t.Run("List returns error on DB failure", func(t *testing.T) {
		svc := closedDB(t)
		_, err := svc.List(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list provider configs")
	})

	t.Run("Get returns error on DB failure", func(t *testing.T) {
		svc := closedDB(t)
		_, err := svc.Get(t.Context(), fake.Lexify("????????"))
		require.Error(t, err)
	})

	t.Run("Create returns error on DB failure", func(t *testing.T) {
		svc := closedDB(t)
		_, err := svc.Create(t.Context(), makeParams())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create provider config")
	})

	t.Run("Update returns error on DB failure", func(t *testing.T) {
		svc := closedDB(t)
		_, err := svc.Update(t.Context(), fake.Lexify("????????"), UpdateProviderConfigParams{
			BaseURL: "https://example.com",
		})
		require.Error(t, err)
	})

	t.Run("Delete returns error on DB failure", func(t *testing.T) {
		svc := closedDB(t)
		err := svc.Delete(t.Context(), fake.Lexify("????????"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete provider config")
	})

	t.Run("Models field", func(t *testing.T) {
		makeModelConfig := func() ModelConfig {
			return ModelConfig{
				Name:        fake.Lexify("model-????????"),
				DisplayName: fake.Company().Name(),
			}
		}

		t.Run("creating provider with models persists and returns models", func(t *testing.T) {
			svc := makeService(t)

			params := makeParams()
			params.Models = []ModelConfig{makeModelConfig(), makeModelConfig()}

			created, err := svc.Create(t.Context(), params)
			require.NoError(t, err)
			require.NotNil(t, created)
			require.Len(t, created.Models, 2)
			assert.Equal(t, params.Models[0].Name, created.Models[0].Name)
			assert.Equal(t, params.Models[0].DisplayName, created.Models[0].DisplayName)
			assert.Equal(t, params.Models[1].Name, created.Models[1].Name)
			assert.Equal(t, params.Models[1].DisplayName, created.Models[1].DisplayName)

			got, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			require.Len(t, got.Models, 2)
			assert.Equal(t, params.Models[0].Name, got.Models[0].Name)
			assert.Equal(t, params.Models[1].Name, got.Models[1].Name)
		})

		t.Run("updating provider models persists and returns updated models", func(t *testing.T) {
			svc := makeService(t)

			params := makeParams()
			params.Models = []ModelConfig{makeModelConfig()}
			created, err := svc.Create(t.Context(), params)
			require.NoError(t, err)

			newModel := makeModelConfig()
			updated, err := svc.Update(t.Context(), created.Name, UpdateProviderConfigParams{
				DisplayName: created.DisplayName,
				BaseURL:     created.BaseURL,
				Models:      []ModelConfig{newModel},
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
			require.Len(t, updated.Models, 1)
			assert.Equal(t, newModel.Name, updated.Models[0].Name)
			assert.Equal(t, newModel.DisplayName, updated.Models[0].DisplayName)

			got, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			require.Len(t, got.Models, 1)
			assert.Equal(t, newModel.Name, got.Models[0].Name)
		})

		t.Run("provider with no models returns empty slice not nil", func(t *testing.T) {
			svc := makeService(t)

			created, err := svc.Create(t.Context(), makeParams())
			require.NoError(t, err)
			assert.NotNil(t, created.Models)
			assert.Empty(t, created.Models)

			got, err := svc.Get(t.Context(), created.Name)
			require.NoError(t, err)
			assert.NotNil(t, got.Models)
			assert.Empty(t, got.Models)
		})
	})
}
