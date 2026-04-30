//go:build !release

package agentapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProviderHandlers(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newServer := func(t *testing.T, svc lp.ProvidersConfigService, modelsLister ModelsLister) (*AgentAPIServer, http.Handler) {
		t.Helper()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: svc,
			AgentProfilesService:   &mockAgentProfilesService{},
			ModelsLister:           modelsLister,
		})
		mux := http.NewServeMux()
		h := HandlerFromMux(srv, mux)
		return srv, h
	}

	newServerWithSvc := func(t *testing.T, svc lp.ProvidersConfigService) (*AgentAPIServer, http.Handler) {
		t.Helper()
		return newServer(t, svc, nil)
	}

	makeProviderConfig := func() lp.ProviderConfig {
		return lp.ProviderConfig{
			Name:        "test-" + fake.Lorem().Word(),
			Type:        lp.ProviderTypeOpenAICompatible,
			DisplayName: fake.Internet().User(),
			BaseURL:     fake.Internet().URL(),
			APIKey:      "sk-" + fake.Lorem().Text(20),
			CreatedAt:   time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
			UpdatedAt:   time.Now().UTC().Truncate(time.Second),
		}
	}

	makeModelConfig := func() lp.ModelConfig {
		return lp.ModelConfig{
			Name:        fake.Lorem().Word() + "-model",
			DisplayName: fake.Lorem().Word() + " Model",
		}
	}

	t.Run("ListProviders", func(t *testing.T) {
		t.Parallel()

		t.Run("returns list of providers", func(t *testing.T) {
			t.Parallel()

			cfg1 := makeProviderConfig()
			cfg2 := makeProviderConfig()

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().List(mock.Anything).Return([]lp.ProviderConfig{cfg1, cfg2}, nil)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var resp ProviderListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Len(t, resp.Providers, 2)
			assert.Equal(t, cfg1.Name, resp.Providers[0].Name)
			assert.Equal(t, cfg2.Name, resp.Providers[1].Name)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().List(mock.Anything).Return(nil, errors.New("storage failure"))

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})

	t.Run("CreateProvider", func(t *testing.T) {
		t.Parallel()

		makeCreateBody := func(cfg lp.ProviderConfig) string {
			displayName := cfg.DisplayName
			return fmt.Sprintf(
				`{"name":%q,"type":%q,"displayName":%q,"baseUrl":%q,"apiKey":%q}`,
				cfg.Name, cfg.Type, displayName, cfg.BaseURL, cfg.APIKey,
			)
		}

		makeCreateBodyWithModels := func(cfg lp.ProviderConfig, models []lp.ModelConfig) string {
			apiModels := make([]ModelConfig, len(models))
			for i, m := range models {
				displayName := m.DisplayName
				apiModels[i] = ModelConfig{Name: m.Name, DisplayName: &displayName}
			}
			body := CreateProviderRequest{
				Name:        cfg.Name,
				Type:        cfg.Type,
				DisplayName: &cfg.DisplayName,
				BaseUrl:     cfg.BaseURL,
				ApiKey:      cfg.APIKey,
				Models:      &apiModels,
			}
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}
			return string(b)
		}

		t.Run("creates and returns 201", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Create(mock.Anything, lp.CreateProviderConfigParams{
				Name:        cfg.Name,
				Type:        cfg.Type,
				DisplayName: cfg.DisplayName,
				BaseURL:     cfg.BaseURL,
				APIKey:      cfg.APIKey,
			}).Return(&cfg, nil)

			_, h := newServerWithSvc(t, svc)

			body := makeCreateBody(cfg)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/providers", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, cfg.Name, resp.Name)
			assert.Equal(t, cfg.Type, resp.Type)
			assert.Equal(t, cfg.BaseURL, resp.BaseUrl)
			assert.Equal(t, maskAPIKey(cfg.APIKey), resp.ApiKeyPreview)
		})

		t.Run("creates with models and response includes models", func(t *testing.T) {
			t.Parallel()

			model1 := makeModelConfig()
			model2 := makeModelConfig()
			cfg := makeProviderConfig()
			cfg.Models = []lp.ModelConfig{model1, model2}

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Create(mock.Anything, lp.CreateProviderConfigParams{
				Name:        cfg.Name,
				Type:        cfg.Type,
				DisplayName: cfg.DisplayName,
				BaseURL:     cfg.BaseURL,
				APIKey:      cfg.APIKey,
				Models:      []lp.ModelConfig{model1, model2},
			}).Return(&cfg, nil)

			_, h := newServerWithSvc(t, svc)

			body := makeCreateBodyWithModels(cfg, []lp.ModelConfig{model1, model2})
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/providers", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Models, 2)
			assert.Equal(t, model1.Name, resp.Models[0].Name)
			assert.Equal(t, model2.Name, resp.Models[1].Name)
		})

		t.Run("returns 400 for malformed JSON body", func(t *testing.T) {
			t.Parallel()

			svc := lp.NewMockProvidersConfigService(t)
			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/providers", strings.NewReader(`{`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusBadRequest, *pd.Status)
		})

		t.Run("returns 409 for duplicate name", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, lp.ErrProviderConfigNameConflict)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPost, "/providers", strings.NewReader(makeCreateBody(cfg)),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusConflict, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusConflict, *pd.Status)
		})

		t.Run("returns 400 for validation error", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			validationErr := errors.New("invalid name pattern")
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Create(mock.Anything, mock.Anything).Return(nil, validationErr)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(
				t.Context(), http.MethodPost, "/providers", strings.NewReader(makeCreateBody(cfg)),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusBadRequest, *pd.Status)
		})
	})

	t.Run("GetProvider", func(t *testing.T) {
		t.Parallel()

		t.Run("returns provider", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Get(mock.Anything, cfg.Name).Return(&cfg, nil)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers/"+cfg.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, cfg.Name, resp.Name)
			assert.Equal(t, maskAPIKey(cfg.APIKey), resp.ApiKeyPreview)
		})

		t.Run("returns provider with models", func(t *testing.T) {
			t.Parallel()

			model1 := makeModelConfig()
			model2 := makeModelConfig()
			cfg := makeProviderConfig()
			cfg.Models = []lp.ModelConfig{model1, model2}

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Get(mock.Anything, cfg.Name).Return(&cfg, nil)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers/"+cfg.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Models, 2)
			assert.Equal(t, model1.Name, resp.Models[0].Name)
			assert.Equal(t, model2.Name, resp.Models[1].Name)
		})

		t.Run("returns 404 for unknown name", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Get(mock.Anything, name).Return(nil, lp.ErrProviderConfigNotFound)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers/"+name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusNotFound, *pd.Status)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Get(mock.Anything, name).Return(nil, errors.New("storage failure"))

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/providers/"+name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})

	t.Run("UpdateProvider", func(t *testing.T) {
		t.Parallel()

		makeUpdateBody := func(baseURL string, displayName *string, apiKey *string) string {
			b := fmt.Sprintf(`{"baseUrl":%q`, baseURL)
			if displayName != nil {
				b += fmt.Sprintf(`,"displayName":%q`, *displayName)
			}
			if apiKey != nil {
				b += fmt.Sprintf(`,"apiKey":%q`, *apiKey)
			}
			b += `}`
			return b
		}

		makeUpdateBodyWithModels := func(baseURL string, models []lp.ModelConfig) string {
			apiModels := make([]ModelConfig, len(models))
			for i, m := range models {
				displayName := m.DisplayName
				apiModels[i] = ModelConfig{Name: m.Name, DisplayName: &displayName}
			}
			body := UpdateProviderRequest{
				BaseUrl: baseURL,
				Models:  &apiModels,
			}
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}
			return string(b)
		}

		t.Run("updates and returns 200", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			newURL := fake.Internet().URL()
			newDisplayName := fake.Internet().User()
			newAPIKey := "sk-new-" + fake.Lorem().Text(10)

			updated := cfg
			updated.BaseURL = newURL
			updated.DisplayName = newDisplayName
			updated.APIKey = newAPIKey

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Update(mock.Anything, cfg.Name, lp.UpdateProviderConfigParams{
				BaseURL:     newURL,
				DisplayName: newDisplayName,
				APIKey:      newAPIKey,
			}).Return(&updated, nil)

			_, h := newServerWithSvc(t, svc)

			body := makeUpdateBody(newURL, &newDisplayName, &newAPIKey)
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+cfg.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, cfg.Name, resp.Name)
			assert.Equal(t, newURL, resp.BaseUrl)
		})

		t.Run("updates models and response reflects update", func(t *testing.T) {
			t.Parallel()

			model1 := makeModelConfig()
			model2 := makeModelConfig()
			cfg := makeProviderConfig()
			newURL := fake.Internet().URL()

			updated := cfg
			updated.BaseURL = newURL
			updated.Models = []lp.ModelConfig{model1, model2}

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Update(mock.Anything, cfg.Name, lp.UpdateProviderConfigParams{
				BaseURL: newURL,
				Models:  []lp.ModelConfig{model1, model2},
			}).Return(&updated, nil)

			_, h := newServerWithSvc(t, svc)

			body := makeUpdateBodyWithModels(newURL, []lp.ModelConfig{model1, model2})
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+cfg.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var resp ProviderResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Models, 2)
			assert.Equal(t, model1.Name, resp.Models[0].Name)
			assert.Equal(t, model2.Name, resp.Models[1].Name)
		})

		t.Run("preserves api key when not provided", func(t *testing.T) {
			t.Parallel()

			cfg := makeProviderConfig()
			newURL := fake.Internet().URL()

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Update(mock.Anything, cfg.Name, lp.UpdateProviderConfigParams{
				BaseURL: newURL,
			}).Return(&cfg, nil)

			_, h := newServerWithSvc(t, svc)

			body := makeUpdateBody(newURL, nil, nil)
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+cfg.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
		})

		t.Run("returns 404 for unknown name", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			newURL := fake.Internet().URL()

			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Update(mock.Anything, name, mock.Anything).Return(nil, lp.ErrProviderConfigNotFound)

			_, h := newServerWithSvc(t, svc)

			body := makeUpdateBody(newURL, nil, nil)
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusNotFound, *pd.Status)
		})

		t.Run("returns 400 for non-not-found error", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			newURL := fake.Internet().URL()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Update(mock.Anything, name, mock.Anything).Return(nil, errors.New("validation failed"))

			_, h := newServerWithSvc(t, svc)

			body := makeUpdateBody(newURL, nil, nil)
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("returns 400 for malformed JSON body", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/providers/"+name,
				strings.NewReader(`{`),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
		})
	})

	t.Run("DeleteProvider", func(t *testing.T) {
		t.Parallel()

		t.Run("returns 204 on success", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Delete(mock.Anything, name).Return(nil)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/providers/"+name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNoContent, rec.Code)
		})

		t.Run("returns 404 for unknown name", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Delete(mock.Anything, name).Return(lp.ErrProviderConfigNotFound)

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/providers/"+name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			assert.Equal(t, http.StatusNotFound, *pd.Status)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()

			name := fake.Lorem().Word()
			svc := lp.NewMockProvidersConfigService(t)
			svc.EXPECT().Delete(mock.Anything, name).Return(errors.New("storage failure"))

			_, h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/providers/"+name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})

	t.Run("ListModels", func(t *testing.T) {
		t.Parallel()

		t.Run("returns all models from all providers", func(t *testing.T) {
			t.Parallel()

			provider1 := "test-" + fake.Lorem().Word()
			provider2 := "test-" + fake.Lorem().Word()
			model1 := rt.ModelInfo{
				Provider:    provider1,
				Name:        fake.Lorem().Word() + "-model",
				DisplayName: fake.Lorem().Word(),
			}
			model2 := rt.ModelInfo{
				Provider:    provider1,
				Name:        fake.Lorem().Word() + "-model",
				DisplayName: fake.Lorem().Word(),
			}
			model3 := rt.ModelInfo{
				Provider: provider2,
				Name:     fake.Lorem().Word() + "-model",
			}

			svc := lp.NewMockProvidersConfigService(t)
			modelsLister := NewMockModelsLister(t)
			modelsLister.EXPECT().ListModels(mock.Anything).Return([]rt.ModelInfo{model1, model2, model3}, nil)

			_, h := newServer(t, svc, modelsLister)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

			var resp ModelListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Models, 3)
			assert.Equal(t, model1.Provider, resp.Models[0].Provider)
			assert.Equal(t, model1.Name, resp.Models[0].Name)
			assert.Equal(t, model2.Provider, resp.Models[1].Provider)
			assert.Equal(t, model3.Provider, resp.Models[2].Provider)
		})

		t.Run("returns empty array when no providers", func(t *testing.T) {
			t.Parallel()

			svc := lp.NewMockProvidersConfigService(t)
			modelsLister := NewMockModelsLister(t)
			modelsLister.EXPECT().ListModels(mock.Anything).Return([]rt.ModelInfo{}, nil)

			_, h := newServer(t, svc, modelsLister)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var resp ModelListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Empty(t, resp.Models)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()

			svc := lp.NewMockProvidersConfigService(t)
			modelsLister := NewMockModelsLister(t)
			modelsLister.EXPECT().ListModels(mock.Anything).Return(nil, errors.New("storage failure"))

			_, h := newServer(t, svc, modelsLister)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/models", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})
}
