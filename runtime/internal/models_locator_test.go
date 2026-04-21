//go:build !release

package internal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/firebase/genkit/go/genkit"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

func TestModelsLocator(t *testing.T) {
	fake := faker.New()

	makeProviderConfig := func(name string) lp.ProviderConfig {
		return lp.ProviderConfig{
			Name:      name,
			Type:      lp.ProviderTypeOpenAICompatible,
			BaseURL:   fake.Internet().URL(),
			APIKey:    fake.Lorem().Word(),
			UpdatedAt: time.Now(),
		}
	}

	makeProviderConfigWithModels := func(name string, models ...lp.ModelConfig) lp.ProviderConfig {
		cfg := makeProviderConfig(name)
		cfg.Models = models
		return cfg
	}

	// makeLocatorParams creates params with injectable genkit init and init call counter.
	makeLocatorParams := func(t *testing.T, svc lp.ProvidersConfigService) (ModelsLocatorParams, *int) {
		t.Helper()
		initCount := 0
		params := ModelsLocatorParams{
			ProvidersSvc: svc,
			Logger:       RootTestLogger().With("test", t.Name()),
			GenkitInitFunc: func(_ context.Context, _ lp.ProviderConfig) (*genkit.Genkit, error) {
				initCount++
				return &genkit.Genkit{}, nil
			},
			ToolStubRegistrar: func(*genkit.Genkit) {},
		}
		return params, &initCount
	}

	// newSvc creates and registers a test mock for lp.ProvidersConfigService.
	newSvc := func(t *testing.T) *mockProvidersConfigService {
		t.Helper()
		m := &mockProvidersConfigService{}
		m.Test(t)
		t.Cleanup(func() { m.AssertExpectations(t) })
		return m
	}

	t.Run("NewModelsLocator", func(t *testing.T) {
		t.Run("nil GenkitInitFunc uses default production init", func(t *testing.T) {
			svc := newSvc(t)
			params := ModelsLocatorParams{
				ProvidersSvc: svc,
				Logger:       RootTestLogger().With("test", t.Name()),
				// GenkitInitFunc is nil intentionally
				ToolStubRegistrar: func(*genkit.Genkit) {},
			}

			locator := NewModelsLocator(params)

			require.NotNil(t, locator)
			// Verify that the internal init function is set (non-nil).
			assert.NotNil(t, locator.genkitInitFunc)
		})
	})

	t.Run("parseModelName", func(t *testing.T) {
		t.Run("valid provider/model returns correct parts", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqName := providerName + "/" + modelName

			provider, model, err := parseModelName(fqName)

			require.NoError(t, err)
			assert.Equal(t, providerName, provider)
			assert.Equal(t, modelName, model)
		})

		t.Run("without slash returns error", func(t *testing.T) {
			_, _, err := parseModelName(fake.Lorem().Word())

			require.Error(t, err)
			require.ErrorContains(t, err, "invalid model name")
		})

		t.Run("empty string returns error", func(t *testing.T) {
			_, _, err := parseModelName("")

			require.Error(t, err)
			require.ErrorContains(t, err, "invalid model name")
		})
	})

	t.Run("ResolveModel", func(t *testing.T) {
		t.Run("resolve model from provider that exists returns LLM adapter", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName
			cfg := makeProviderConfig(providerName)

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg, nil).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			llm, err := locator.ResolveModel(t.Context(), fqModelName)

			require.NoError(t, err)
			require.NotNil(t, llm)
			assert.Equal(t, fqModelName, llm.Name())
		})

		t.Run("resolve model from unknown provider returns error", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(nil, lp.ErrProviderConfigNotFound).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			_, err := locator.ResolveModel(t.Context(), fqModelName)

			require.Error(t, err)
			require.ErrorIs(t, err, lp.ErrProviderConfigNotFound)
		})

		t.Run("resolve model with invalid name returns error", func(t *testing.T) {
			svc := newSvc(t)
			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			_, err := locator.ResolveModel(t.Context(), fake.Lorem().Word())

			require.Error(t, err)
			require.ErrorContains(t, err, "invalid model name")
		})

		t.Run("resolve model after provider update creates new genkit instance", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName

			updatedAt1 := time.Now().Add(-time.Hour)
			updatedAt2 := time.Now()

			cfg1 := makeProviderConfig(providerName)
			cfg1.UpdatedAt = updatedAt1
			cfg2 := makeProviderConfig(providerName)
			cfg2.UpdatedAt = updatedAt2

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg1, nil).Once()
			svc.On("Get", mock.Anything, providerName).Return(&cfg2, nil).Once()

			params, initCount := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			_, err := locator.ResolveModel(t.Context(), fqModelName)
			require.NoError(t, err)

			_, err = locator.ResolveModel(t.Context(), fqModelName)
			require.NoError(t, err)

			assert.Equal(t, 2, *initCount, "new genkit instance expected for updated provider")
		})

		t.Run("resolve model with same UpdatedAt reuses cached genkit instance", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName
			cfg := makeProviderConfig(providerName)

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg, nil).Times(3)

			params, initCount := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			for range 3 {
				_, err := locator.ResolveModel(t.Context(), fqModelName)
				require.NoError(t, err)
			}

			assert.Equal(t, 1, *initCount, "cached genkit instance should be reused")
		})

		t.Run("concurrent resolve calls for same provider only inits once", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName
			cfg := makeProviderConfig(providerName)

			const numGoroutines = 10
			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg, nil).Times(numGoroutines)

			params, initCount := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			var wg sync.WaitGroup
			errs := make([]error, numGoroutines)
			for i := range numGoroutines {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					_, errs[idx] = locator.ResolveModel(t.Context(), fqModelName)
				}(i)
			}
			wg.Wait()

			for _, err := range errs {
				require.NoError(t, err)
			}
			assert.Equal(t, 1, *initCount, "mutex should prevent multiple genkit.Init calls")
		})

		t.Run("genkit init error propagates from ResolveModel", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName
			cfg := makeProviderConfig(providerName)

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg, nil).Once()

			initErr := fmt.Errorf("genkit init failed: %w", errors.New("connection refused"))
			params := ModelsLocatorParams{
				ProvidersSvc: svc,
				Logger:       RootTestLogger().With("test", t.Name()),
				GenkitInitFunc: func(_ context.Context, _ lp.ProviderConfig) (*genkit.Genkit, error) {
					return nil, initErr
				},
				ToolStubRegistrar: func(*genkit.Genkit) {},
			}
			locator := NewModelsLocator(params)

			_, err := locator.ResolveModel(t.Context(), fqModelName)

			require.Error(t, err)
			require.ErrorContains(t, err, "genkit init failed")
		})

		t.Run("tool stubs registered on each new genkit instance", func(t *testing.T) {
			providerName := fake.Lorem().Word()
			modelName := fake.Lorem().Word()
			fqModelName := providerName + "/" + modelName

			updatedAt1 := time.Now().Add(-time.Hour)
			updatedAt2 := time.Now()
			cfg1 := makeProviderConfig(providerName)
			cfg1.UpdatedAt = updatedAt1
			cfg2 := makeProviderConfig(providerName)
			cfg2.UpdatedAt = updatedAt2

			svc := newSvc(t)
			svc.On("Get", mock.Anything, providerName).Return(&cfg1, nil).Once()
			svc.On("Get", mock.Anything, providerName).Return(&cfg2, nil).Once()

			stubCallCount := 0
			params := ModelsLocatorParams{
				ProvidersSvc: svc,
				Logger:       RootTestLogger().With("test", t.Name()),
				GenkitInitFunc: func(_ context.Context, _ lp.ProviderConfig) (*genkit.Genkit, error) {
					return &genkit.Genkit{}, nil
				},
				ToolStubRegistrar: func(_ *genkit.Genkit) {
					stubCallCount++
				},
			}
			locator := NewModelsLocator(params)

			_, err := locator.ResolveModel(t.Context(), fqModelName)
			require.NoError(t, err)

			_, err = locator.ResolveModel(t.Context(), fqModelName)
			require.NoError(t, err)

			assert.Equal(t, 2, stubCallCount, "tool stubs must be registered on every new genkit instance")
		})
	})

	t.Run("ListModels", func(t *testing.T) {
		t.Run("returns models from all providers", func(t *testing.T) {
			provider1Name := fake.Lorem().Word()
			provider2Name := fake.Lorem().Word()

			model1 := lp.ModelConfig{Name: fake.Lorem().Word(), DisplayName: fake.Lorem().Word()}
			model2 := lp.ModelConfig{Name: fake.Lorem().Word(), DisplayName: fake.Lorem().Word()}
			model3 := lp.ModelConfig{Name: fake.Lorem().Word(), DisplayName: fake.Lorem().Word()}

			providers := []lp.ProviderConfig{
				makeProviderConfigWithModels(provider1Name, model1, model2),
				makeProviderConfigWithModels(provider2Name, model3),
			}

			svc := newSvc(t)
			svc.On("List", mock.Anything).Return(providers, nil).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			models, err := locator.ListModels(t.Context())

			require.NoError(t, err)
			require.Len(t, models, 3)

			modelSet := make(map[string]ModelInfo, len(models))
			for _, m := range models {
				modelSet[m.Provider+"/"+m.Name] = m
			}

			assert.Equal(t,
				ModelInfo{Provider: provider1Name, Name: model1.Name, DisplayName: model1.DisplayName},
				modelSet[provider1Name+"/"+model1.Name])
			assert.Equal(t,
				ModelInfo{Provider: provider1Name, Name: model2.Name, DisplayName: model2.DisplayName},
				modelSet[provider1Name+"/"+model2.Name])
			assert.Equal(t,
				ModelInfo{Provider: provider2Name, Name: model3.Name, DisplayName: model3.DisplayName},
				modelSet[provider2Name+"/"+model3.Name])
		})

		t.Run("no providers returns empty slice", func(t *testing.T) {
			svc := newSvc(t)
			svc.On("List", mock.Anything).Return([]lp.ProviderConfig{}, nil).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			models, err := locator.ListModels(t.Context())

			require.NoError(t, err)
			assert.Empty(t, models)
		})

		t.Run("propagates list error", func(t *testing.T) {
			svc := newSvc(t)
			listErr := fmt.Errorf("db error: %w", lp.ErrProviderConfigNotFound)
			svc.On("List", mock.Anything).Return(nil, listErr).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			_, err := locator.ListModels(t.Context())

			require.Error(t, err)
			require.ErrorContains(t, err, "db error")
		})

		t.Run("provider with no models contributes zero entries", func(t *testing.T) {
			provider1Name := fake.Lorem().Word()
			provider2Name := fake.Lorem().Word()

			model1 := lp.ModelConfig{Name: fake.Lorem().Word()}
			providers := []lp.ProviderConfig{
				makeProviderConfigWithModels(provider1Name),
				makeProviderConfigWithModels(provider2Name, model1),
			}

			svc := newSvc(t)
			svc.On("List", mock.Anything).Return(providers, nil).Once()

			params, _ := makeLocatorParams(t, svc)
			locator := NewModelsLocator(params)

			models, err := locator.ListModels(t.Context())

			require.NoError(t, err)
			require.Len(t, models, 1)
			assert.Equal(t, provider2Name, models[0].Provider)
			assert.Equal(t, model1.Name, models[0].Name)
		})
	})
}

// mockProvidersConfigService is a testify mock for ProvidersConfigService, local to this test file.
type mockProvidersConfigService struct {
	mock.Mock
}

func (m *mockProvidersConfigService) List(ctx context.Context) ([]lp.ProviderConfig, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]lp.ProviderConfig), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockProvidersConfigService) Get(ctx context.Context, name string) (*lp.ProviderConfig, error) {
	args := m.Called(ctx, name)
	if v := args.Get(0); v != nil {
		return v.(*lp.ProviderConfig), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockProvidersConfigService) Create(
	ctx context.Context, params lp.CreateProviderConfigParams,
) (*lp.ProviderConfig, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.(*lp.ProviderConfig), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockProvidersConfigService) Update(
	ctx context.Context, name string, params lp.UpdateProviderConfigParams,
) (*lp.ProviderConfig, error) {
	args := m.Called(ctx, name, params)
	if v := args.Get(0); v != nil {
		return v.(*lp.ProviderConfig), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockProvidersConfigService) Delete(ctx context.Context, name string) error {
	return m.Called(ctx, name).Error(0)
}
