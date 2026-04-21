package llmproviders

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

func TestFileProvidersConfigService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T) *FileProvidersConfigService {
		t.Helper()
		svc, err := NewFileProvidersConfigService(
			t.TempDir(),
			testLogger(t),
		)
		require.NoError(t, err)
		return svc
	}

	makeParams := func() CreateProviderConfigParams {
		return CreateProviderConfigParams{
			Name:        "provider-" + fake.Lorem().Word(),
			Type:        ProviderTypeOpenAICompatible,
			DisplayName: fake.Company().Name(),
			BaseURL:     "https://" + fake.Internet().Domain(),
			APIKey:      "sk-" + fake.Lorem().Text(20),
		}
	}

	t.Run("NewFileProvidersConfigService", func(t *testing.T) {
		t.Run("creates service and providers directory", func(t *testing.T) {
			baseDir := t.TempDir()
			svc, err := NewFileProvidersConfigService(baseDir, nil)
			require.NoError(t, err)
			require.NotNil(t, svc)
		})

		t.Run("returns error for empty baseDir", func(t *testing.T) {
			svc, err := NewFileProvidersConfigService("", nil)
			require.Error(t, err)
			assert.Nil(t, svc)
			assert.Contains(t, err.Error(), "base_dir")
		})
	})

	t.Run("List", func(t *testing.T) {
		t.Run("returns empty slice when providers directory was removed", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			providersDir := filepath.Join(svc.baseDir, "providers")
			require.NoError(t, os.RemoveAll(providersDir))

			result, err := svc.List(ctx)
			require.NoError(t, err)
			assert.Empty(t, result)
		})

		t.Run("returns empty slice when no providers exist", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			result, err := svc.List(ctx)
			require.NoError(t, err)
			assert.Empty(t, result)
		})

		t.Run("includes legacy .yml when no .yaml exists for stem", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			const name = "yml-list-only"
			yml := `name: yml-list-only
type: openai-compatible
displayName: ListYML
baseUrl: https://list-yml.example
apiKey: sk-list-yml
models: []
createdAt: 2024-06-01T12:00:00Z
updatedAt: 2024-06-01T12:00:00Z
`
			require.NoError(t, os.WriteFile(filepath.Join(svc.baseDir, "providers", name+".yml"), []byte(yml), 0600))

			result, err := svc.List(ctx)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, name, result[0].Name)
			assert.Equal(t, "sk-list-yml", result[0].APIKey)
		})

		t.Run("returns all providers sorted by createdAt ascending", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			p1params := makeParams()
			p2params := makeParams()
			p3params := makeParams()

			p1, err := svc.Create(ctx, p1params)
			require.NoError(t, err)
			// small delay so timestamps differ
			time.Sleep(2 * time.Millisecond)
			p2, err := svc.Create(ctx, p2params)
			require.NoError(t, err)
			time.Sleep(2 * time.Millisecond)
			p3, err := svc.Create(ctx, p3params)
			require.NoError(t, err)

			result, err := svc.List(ctx)
			require.NoError(t, err)
			require.Len(t, result, 3)

			assert.Equal(t, p1.Name, result[0].Name)
			assert.Equal(t, p2.Name, result[1].Name)
			assert.Equal(t, p3.Name, result[2].Name)
			assert.False(t, result[0].CreatedAt.After(result[1].CreatedAt))
			assert.False(t, result[1].CreatedAt.After(result[2].CreatedAt))
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("returns provider by name", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			params := makeParams()
			created, err := svc.Create(ctx, params)
			require.NoError(t, err)

			got, err := svc.Get(ctx, created.Name)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, created.Name, got.Name)
			assert.Equal(t, created.Type, got.Type)
			assert.Equal(t, created.DisplayName, got.DisplayName)
			assert.Equal(t, created.BaseURL, got.BaseURL)
			assert.Equal(t, created.APIKey, got.APIKey)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			_, err := svc.Get(ctx, "nonexistent-"+fake.Lorem().Word())
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
	})

	t.Run("Create", func(t *testing.T) {
		t.Run("creates provider and returns it with timestamps", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()

			before := time.Now().Truncate(time.Millisecond)
			result, err := svc.Create(ctx, params)
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
			ctx := t.Context()
			params := makeParams()

			created, err := svc.Create(ctx, params)
			require.NoError(t, err)

			got, err := svc.Get(ctx, created.Name)
			require.NoError(t, err)
			assert.Equal(t, created.Name, got.Name)
			assert.Equal(t, created.APIKey, got.APIKey)
		})

		t.Run("writes provider file as .yaml on disk", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()

			created, err := svc.Create(ctx, params)
			require.NoError(t, err)

			path := filepath.Join(svc.baseDir, "providers", created.Name+".yaml")
			st, err := os.Stat(path)
			require.NoError(t, err)
			assert.False(t, st.IsDir())
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(raw), "apiKey:")
		})

		t.Run("returns ErrProviderConfigNameConflict for duplicate name", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()

			_, err := svc.Create(ctx, params)
			require.NoError(t, err)

			_, err = svc.Create(ctx, params)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNameConflict)
		})

		t.Run("returns ErrProviderConfigNameConflict when legacy .yml exists without .yaml", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			ymlPath := filepath.Join(svc.baseDir, "providers", params.Name+".yml")
			legacy := `name: ` + params.Name + `
type: openai-compatible
displayName: X
baseUrl: https://x.example
apiKey: sk-x
models: []
createdAt: 2024-06-01T12:00:00Z
updatedAt: 2024-06-01T12:00:00Z
`
			require.NoError(t, os.WriteFile(ymlPath, []byte(legacy), 0600))

			_, err := svc.Create(ctx, params)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNameConflict)
		})

		t.Run("rejects invalid name pattern - starts with digit", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			params.Name = "1invalid"

			_, err := svc.Create(ctx, params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name")
		})

		t.Run("rejects invalid name pattern - uppercase", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			params.Name = "InvalidName"

			_, err := svc.Create(ctx, params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name")
		})

		t.Run("accepts valid name with hyphens and digits", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			params.Name = "my-provider-1"

			result, err := svc.Create(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, "my-provider-1", result.Name)
		})

		t.Run("rejects unsupported provider type", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			params.Type = "unsupported-type-" + fake.Lorem().Word()

			_, err := svc.Create(ctx, params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "type")
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("updates provider fields", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			created, err := svc.Create(ctx, makeParams())
			require.NoError(t, err)

			time.Sleep(2 * time.Millisecond)
			newDisplayName := fake.Company().Name()
			newBaseURL := "https://" + fake.Internet().Domain()
			newAPIKey := "sk-new-" + fake.Lorem().Text(20)

			updated, err := svc.Update(ctx, created.Name, UpdateProviderConfigParams{
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
			ctx := t.Context()

			created, err := svc.Create(ctx, makeParams())
			require.NoError(t, err)
			originalKey := created.APIKey

			updated, err := svc.Update(ctx, created.Name, UpdateProviderConfigParams{
				BaseURL: "https://" + fake.Internet().Domain(),
				APIKey:  "",
			})
			require.NoError(t, err)
			assert.Equal(t, originalKey, updated.APIKey)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			_, err := svc.Update(ctx, "nonexistent-"+fake.Lorem().Word(), UpdateProviderConfigParams{
				BaseURL: "https://" + fake.Internet().Domain(),
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("deletes provider", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			created, err := svc.Create(ctx, makeParams())
			require.NoError(t, err)

			err = svc.Delete(ctx, created.Name)
			require.NoError(t, err)

			_, err = svc.Get(ctx, created.Name)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})

		t.Run("returns ErrProviderConfigNotFound for unknown name", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			err := svc.Delete(ctx, "nonexistent-"+fake.Lorem().Word())
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrProviderConfigNotFound)
		})

		t.Run("returns error when file cannot be removed", func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("chmod permissions differ on Windows")
			}
			svc := makeService(t)
			ctx := t.Context()

			created, err := svc.Create(ctx, makeParams())
			require.NoError(t, err)

			path := svc.providerPath(created.Name)
			require.NoError(t, os.Remove(path))
			require.NoError(t, os.Mkdir(path, 0750))
			require.NoError(t, os.WriteFile(filepath.Join(path, "child"), []byte("x"), 0600))
			t.Cleanup(func() { _ = os.RemoveAll(path) })

			err = svc.Delete(ctx, created.Name)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "remove provider file")
		})
	})

	t.Run("readProviderFile maps missing file to ErrProviderConfigNotFound", func(t *testing.T) {
		svc := makeService(t)
		t.Run("canonical yaml suffix", func(t *testing.T) {
			missing := filepath.Join(svc.baseDir, "providers", "absent-"+fake.Lorem().Word()+".yaml")
			_, err := svc.readProviderFile(missing)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrProviderConfigNotFound)
		})
		t.Run("non yaml suffix uses basename without extension in not-found name", func(t *testing.T) {
			missing := filepath.Join(svc.baseDir, "providers", "odd-"+fake.Lorem().Word()+".dat")
			_, err := svc.readProviderFile(missing)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrProviderConfigNotFound)
			assert.Contains(t, err.Error(), "odd-")
		})
	})

	t.Run("NewFileProvidersConfigService fails when dir cannot be created", func(t *testing.T) {
		tmp := t.TempDir()
		blocked := filepath.Join(tmp, "not-a-dir")
		require.NoError(t, os.WriteFile(blocked, []byte("x"), 0600))

		_, err := NewFileProvidersConfigService(filepath.Join(blocked, "nested"), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create providers dir")
	})

	t.Run("List returns error when providers directory cannot be read", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ on Windows")
		}
		svc := makeService(t)
		ctx := t.Context()

		providersDir := filepath.Join(svc.baseDir, "providers")
		require.NoError(t, os.Chmod(providersDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(providersDir, 0750) })

		_, err := svc.List(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read providers dir")
	})

	t.Run("List skips directories and non-yaml provider files", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		providersDir := filepath.Join(svc.baseDir, "providers")
		require.NoError(t, os.WriteFile(filepath.Join(providersDir, "notes.txt"), []byte("x"), 0600))
		require.NoError(t, os.Mkdir(filepath.Join(providersDir, "subdir"), 0750))

		_, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		result, err := svc.List(ctx)
		require.NoError(t, err)
		require.Len(t, result, 1)
	})

	t.Run("List returns error when provider file is corrupt YAML", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		path := svc.providerPath(created.Name)
		require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

		_, err = svc.List(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse provider file")
	})

	t.Run("YAML file naming", func(t *testing.T) {
		t.Run("List prefers .yaml over same-named .yml", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			params := makeParams()
			params.Name = "same-stem"
			created, err := svc.Create(ctx, params)
			require.NoError(t, err)

			ymlPath := filepath.Join(svc.baseDir, "providers", created.Name+".yml")
			wrongYML := `name: ` + created.Name + `
type: openai-compatible
displayName: wrong
baseUrl: https://wrong.example
apiKey: sk-wrong
models: []
createdAt: 2020-01-01T00:00:00Z
updatedAt: 2020-01-01T00:00:00Z
`
			require.NoError(t, os.WriteFile(ymlPath, []byte(wrongYML), 0600))

			list, err := svc.List(ctx)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, created.APIKey, list[0].APIKey)
		})

		t.Run("Get reads legacy .yml when .yaml is absent", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			const name = "yml-only"
			yml := `name: yml-only
type: openai-compatible
displayName: Legacy
baseUrl: https://legacy.example/v1
apiKey: sk-legacy
models: []
createdAt: 2024-06-01T12:00:00Z
updatedAt: 2024-06-01T12:00:00Z
`
			ymlPath := filepath.Join(svc.baseDir, "providers", name+".yml")
			require.NoError(t, os.WriteFile(ymlPath, []byte(yml), 0600))

			got, err := svc.Get(ctx, name)
			require.NoError(t, err)
			assert.Equal(t, "sk-legacy", got.APIKey)
			assert.Equal(t, name, got.Name)
		})

		t.Run("Update migrates legacy .yml to canonical .yaml", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()
			const name = "migrate-me"
			yml := `name: migrate-me
type: openai-compatible
displayName: Old
baseUrl: https://old.example/v1
apiKey: sk-old
models: []
createdAt: 2024-06-01T12:00:00Z
updatedAt: 2024-06-01T12:00:00Z
`
			ymlPath := filepath.Join(svc.baseDir, "providers", name+".yml")
			require.NoError(t, os.WriteFile(ymlPath, []byte(yml), 0600))

			_, err := svc.Update(ctx, name, UpdateProviderConfigParams{
				DisplayName: "New",
				BaseURL:     "https://new.example/v1",
				APIKey:      "sk-new",
			})
			require.NoError(t, err)

			yamlPath := filepath.Join(svc.baseDir, "providers", name+".yaml")
			_, err = os.Stat(yamlPath)
			require.NoError(t, err)
			_, err = os.Stat(ymlPath)
			assert.True(t, os.IsNotExist(err))

			got, err := svc.Get(ctx, name)
			require.NoError(t, err)
			assert.Equal(t, "sk-new", got.APIKey)
			assert.Equal(t, "New", got.DisplayName)
		})
	})

	t.Run("Get returns error when provider file is corrupt YAML", func(t *testing.T) {
		svc := makeService(t)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		path := svc.providerPath(created.Name)
		require.NoError(t, os.WriteFile(path, []byte("{bad: yaml: [[["), 0600))

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse provider file")
	})

	t.Run("Get returns error when file is not readable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ on Windows")
		}
		svc := makeService(t)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		path := svc.providerPath(created.Name)
		require.NoError(t, os.Chmod(path, 0000))
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read provider file")
	})

	t.Run("Get returns stat error when providers directory is inaccessible", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ on Windows")
		}
		svc := makeService(t)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		providersDir := filepath.Join(svc.baseDir, "providers")
		require.NoError(t, os.Chmod(providersDir, 0000))
		t.Cleanup(func() { _ = os.Chmod(providersDir, 0750) })

		_, err = svc.Get(ctx, created.Name)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat provider file")
	})

	t.Run("Create returns error when file cannot be written", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ on Windows")
		}
		svc := makeService(t)
		ctx := t.Context()

		providersDir := filepath.Join(svc.baseDir, "providers")
		require.NoError(t, os.Chmod(providersDir, 0500))
		t.Cleanup(func() { _ = os.Chmod(providersDir, 0750) })

		params := makeParams()
		_, err := svc.Create(ctx, params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write provider file")
	})

	t.Run("Update returns error when file cannot be written", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod permissions differ on Windows")
		}
		svc := makeService(t)
		ctx := t.Context()

		created, err := svc.Create(ctx, makeParams())
		require.NoError(t, err)

		path := svc.providerPath(created.Name)
		require.NoError(t, os.Chmod(path, 0400))
		t.Cleanup(func() { _ = os.Chmod(path, 0600) })

		_, err = svc.Update(ctx, created.Name, UpdateProviderConfigParams{
			BaseURL: "https://" + fake.Internet().Domain(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write provider file")
	})

	t.Run("Models field", func(t *testing.T) {
		makeModelConfig := func() ModelConfig {
			return ModelConfig{
				Name:        fake.Lorem().Word() + "-" + fake.Lorem().Word(),
				DisplayName: fake.Company().Name(),
			}
		}

		t.Run("creating provider with models persists and returns models", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			params := makeParams()
			params.Models = []ModelConfig{makeModelConfig(), makeModelConfig()}

			created, err := svc.Create(ctx, params)
			require.NoError(t, err)
			require.NotNil(t, created)
			require.Len(t, created.Models, 2)
			assert.Equal(t, params.Models[0].Name, created.Models[0].Name)
			assert.Equal(t, params.Models[0].DisplayName, created.Models[0].DisplayName)
			assert.Equal(t, params.Models[1].Name, created.Models[1].Name)
			assert.Equal(t, params.Models[1].DisplayName, created.Models[1].DisplayName)

			got, err := svc.Get(ctx, created.Name)
			require.NoError(t, err)
			require.Len(t, got.Models, 2)
			assert.Equal(t, params.Models[0].Name, got.Models[0].Name)
			assert.Equal(t, params.Models[1].Name, got.Models[1].Name)
		})

		t.Run("updating provider models persists and returns updated models", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			params := makeParams()
			params.Models = []ModelConfig{makeModelConfig()}
			created, err := svc.Create(ctx, params)
			require.NoError(t, err)

			newModel := makeModelConfig()
			updated, err := svc.Update(ctx, created.Name, UpdateProviderConfigParams{
				DisplayName: created.DisplayName,
				BaseURL:     created.BaseURL,
				Models:      []ModelConfig{newModel},
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
			require.Len(t, updated.Models, 1)
			assert.Equal(t, newModel.Name, updated.Models[0].Name)
			assert.Equal(t, newModel.DisplayName, updated.Models[0].DisplayName)

			got, err := svc.Get(ctx, created.Name)
			require.NoError(t, err)
			require.Len(t, got.Models, 1)
			assert.Equal(t, newModel.Name, got.Models[0].Name)
		})

		t.Run("provider with no models returns empty slice not nil", func(t *testing.T) {
			svc := makeService(t)
			ctx := t.Context()

			params := makeParams()
			created, err := svc.Create(ctx, params)
			require.NoError(t, err)
			assert.NotNil(t, created.Models)
			assert.Empty(t, created.Models)

			got, err := svc.Get(ctx, created.Name)
			require.NoError(t, err)
			assert.NotNil(t, got.Models)
			assert.Empty(t, got.Models)
		})
	})
}
