package agentapi

import (
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

func TestProviderMapper(t *testing.T) {
	fake := faker.New()

	t.Run("maskAPIKey", func(t *testing.T) {
		t.Run("empty key returns empty string", func(t *testing.T) {
			assert.Empty(t, maskAPIKey(""))
		})

		t.Run("key shorter than 4 chars returns ...", func(t *testing.T) {
			assert.Equal(t, "...", maskAPIKey("abc"))
			assert.Equal(t, "...", maskAPIKey("a"))
			assert.Equal(t, "...", maskAPIKey("ab"))
		})

		t.Run("key exactly 4 chars returns ...XXXX", func(t *testing.T) {
			assert.Equal(t, "...abcd", maskAPIKey("abcd"))
		})

		t.Run("key longer than 4 chars returns ...last4", func(t *testing.T) {
			assert.Equal(t, "...xYz1", maskAPIKey("sk-somekey-xYz1"))
			assert.Equal(t, "...4321", maskAPIKey("sk-abc-4321"))
		})
	})

	t.Run("mapProviderConfigToResponse", func(t *testing.T) {
		t.Run("maps all fields correctly", func(t *testing.T) {
			displayName := fake.Internet().User()
			createdAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
			updatedAt := time.Now().UTC().Truncate(time.Second)
			apiKey := "sk-" + fake.Lorem().Text(20)
			modelName := fake.Lorem().Word() + "-model"
			modelDisplayName := fake.Lorem().Word() + " Model"

			cfg := lp.ProviderConfig{
				Name:        fake.Lorem().Word(),
				Type:        lp.ProviderTypeOpenAICompatible,
				DisplayName: displayName,
				BaseURL:     fake.Internet().URL(),
				APIKey:      apiKey,
				Models: []lp.ModelConfig{
					{Name: modelName, DisplayName: modelDisplayName, Summarization: true},
				},
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}

			resp := mapProviderConfigToResponse(cfg)

			assert.Equal(t, cfg.Name, resp.Name)
			assert.Equal(t, cfg.Type, resp.Type)
			assert.Equal(t, &cfg.DisplayName, resp.DisplayName)
			assert.Equal(t, cfg.BaseURL, resp.BaseUrl)
			assert.Equal(t, maskAPIKey(apiKey), resp.ApiKeyPreview)
			assert.Equal(t, createdAt, resp.CreatedAt)
			assert.Equal(t, updatedAt, resp.UpdatedAt)
			require.Len(t, resp.Models, 1)
			assert.Equal(t, modelName, resp.Models[0].Name)
			assert.Equal(t, &modelDisplayName, resp.Models[0].DisplayName)
			require.NotNil(t, resp.Models[0].Summarization)
			assert.True(t, *resp.Models[0].Summarization)
		})

		t.Run("empty models returns empty slice", func(t *testing.T) {
			cfg := lp.ProviderConfig{
				Name:      fake.Lorem().Word(),
				Type:      lp.ProviderTypeOpenAICompatible,
				BaseURL:   fake.Internet().URL(),
				APIKey:    "sk-" + fake.Lorem().Text(20),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			resp := mapProviderConfigToResponse(cfg)

			assert.Empty(t, resp.Models)
		})

		t.Run("empty display name maps to nil", func(t *testing.T) {
			cfg := lp.ProviderConfig{
				Name:        fake.Lorem().Word(),
				Type:        lp.ProviderTypeOpenAICompatible,
				DisplayName: "",
				BaseURL:     fake.Internet().URL(),
				APIKey:      "sk-" + fake.Lorem().Text(20),
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}

			resp := mapProviderConfigToResponse(cfg)

			assert.Nil(t, resp.DisplayName)
		})

		t.Run("masks api key in response", func(t *testing.T) {
			apiKey := "sk-test-abcd"
			cfg := lp.ProviderConfig{
				Name:      fake.Lorem().Word(),
				Type:      lp.ProviderTypeOpenAICompatible,
				BaseURL:   fake.Internet().URL(),
				APIKey:    apiKey,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}

			resp := mapProviderConfigToResponse(cfg)

			assert.Equal(t, "...abcd", resp.ApiKeyPreview)
			// full key must not appear in response
			assert.NotContains(t, resp.ApiKeyPreview, "sk-test-")
		})
	})

	t.Run("mapModelConfigToAPI", func(t *testing.T) {
		t.Run("maps name and display name", func(t *testing.T) {
			name := fake.Lorem().Word() + "-model"
			displayName := fake.Lorem().Word() + " Model"
			m := lp.ModelConfig{Name: name, DisplayName: displayName}

			mc := mapModelConfigToAPI(m)

			assert.Equal(t, name, mc.Name)
			require.NotNil(t, mc.DisplayName)
			assert.Equal(t, displayName, *mc.DisplayName)
		})

		t.Run("empty display name maps to nil", func(t *testing.T) {
			m := lp.ModelConfig{Name: fake.Lorem().Word()}

			mc := mapModelConfigToAPI(m)

			assert.Nil(t, mc.DisplayName)
		})

		t.Run("summarization false maps to pointer false", func(t *testing.T) {
			m := lp.ModelConfig{Name: fake.Lorem().Word() + "-model", Summarization: false}

			mc := mapModelConfigToAPI(m)

			require.NotNil(t, mc.Summarization)
			assert.False(t, *mc.Summarization)
		})

		t.Run("summarization true maps to pointer true", func(t *testing.T) {
			m := lp.ModelConfig{Name: fake.Lorem().Word() + "-model", Summarization: true}

			mc := mapModelConfigToAPI(m)

			require.NotNil(t, mc.Summarization)
			assert.True(t, *mc.Summarization)
		})
	})

	t.Run("mapAPIModelConfigToInternal", func(t *testing.T) {
		t.Run("maps name and display name", func(t *testing.T) {
			displayName := fake.Lorem().Word() + " Model"
			mc := ModelConfig{Name: fake.Lorem().Word(), DisplayName: &displayName}

			m := mapAPIModelConfigToInternal(mc)

			assert.Equal(t, mc.Name, m.Name)
			assert.Equal(t, displayName, m.DisplayName)
		})

		t.Run("nil display name maps to empty string", func(t *testing.T) {
			mc := ModelConfig{Name: fake.Lorem().Word()}

			m := mapAPIModelConfigToInternal(mc)

			assert.Empty(t, m.DisplayName)
		})

		t.Run("nil summarization maps to false", func(t *testing.T) {
			mc := ModelConfig{Name: fake.Lorem().Word() + "-model"}

			m := mapAPIModelConfigToInternal(mc)

			assert.False(t, m.Summarization)
		})

		t.Run("explicit summarization maps through", func(t *testing.T) {
			t.Run("true", func(t *testing.T) {
				v := true
				mc := ModelConfig{Name: fake.Lorem().Word() + "-model", Summarization: &v}

				m := mapAPIModelConfigToInternal(mc)

				assert.True(t, m.Summarization)
			})
			t.Run("false", func(t *testing.T) {
				v := false
				mc := ModelConfig{Name: fake.Lorem().Word() + "-model", Summarization: &v}

				m := mapAPIModelConfigToInternal(mc)

				assert.False(t, m.Summarization)
			})
		})
	})

	t.Run("mapProviderListToResponse", func(t *testing.T) {
		t.Run("empty slice returns empty providers list", func(t *testing.T) {
			resp := mapProviderListToResponse([]lp.ProviderConfig{})
			assert.Empty(t, resp.Providers)
		})

		t.Run("maps all configs to responses", func(t *testing.T) {
			configs := []lp.ProviderConfig{
				{
					Name:      fake.Lorem().Word() + "1",
					Type:      lp.ProviderTypeOpenAICompatible,
					BaseURL:   fake.Internet().URL(),
					APIKey:    "sk-" + fake.Lorem().Text(20),
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
				{
					Name:      fake.Lorem().Word() + "2",
					Type:      lp.ProviderTypeOpenAICompatible,
					BaseURL:   fake.Internet().URL(),
					APIKey:    "sk-" + fake.Lorem().Text(20),
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
			}

			resp := mapProviderListToResponse(configs)

			assert.Len(t, resp.Providers, 2)
			assert.Equal(t, configs[0].Name, resp.Providers[0].Name)
			assert.Equal(t, configs[1].Name, resp.Providers[1].Name)
		})
	})
}
