//go:build !release

package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/agent"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockAgentProfilesService struct {
	mock.Mock
}

func (m *mockAgentProfilesService) List(ctx context.Context) ([]ap.AgentProfile, error) {
	args := m.Called(ctx)
	return args.Get(0).([]ap.AgentProfile), args.Error(1)
}

func (m *mockAgentProfilesService) Get(ctx context.Context, name string) (*ap.AgentProfile, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ap.AgentProfile), args.Error(1)
}

func (m *mockAgentProfilesService) Create(
	ctx context.Context,
	params ap.CreateAgentProfileParams,
) (*ap.AgentProfile, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ap.AgentProfile), args.Error(1)
}

func (m *mockAgentProfilesService) Update(
	ctx context.Context,
	name string,
	params ap.UpdateAgentProfileParams,
) (*ap.AgentProfile, error) {
	args := m.Called(ctx, name, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ap.AgentProfile), args.Error(1)
}

func (m *mockAgentProfilesService) Delete(ctx context.Context, name string) error {
	args := m.Called(ctx, name)
	return args.Error(0)
}

func (m *mockAgentProfilesService) AutoMigrate() error {
	args := m.Called()
	return args.Error(0)
}

func TestAgentProfileHandlers(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newServerWithSvc := func(t *testing.T, svc ap.AgentProfilesService) http.Handler {
		t.Helper()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   svc,
		})
		return HandlerFromMux(srv, http.NewServeMux())
	}

	makeProfile := func() ap.AgentProfile {
		return ap.AgentProfile{
			Name:         "profile-" + fake.Lorem().Word(),
			DisplayName:  fake.Lorem().Word(),
			Role:         fake.Lorem().Word(),
			Instructions: fake.Lorem().Sentence(5),
			ToolRefs:     []string{"tool-a", "tool-b"},
			ExecutionSettings: ap.ExecutionSettings{
				DefaultModel: "openai/gpt-4.1",
			},
			CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
			UpdatedAt: time.Now().UTC().Truncate(time.Second),
		}
	}

	t.Run("ListAgentProfiles", func(t *testing.T) {
		t.Parallel()

		t.Run("returns list in service order", func(t *testing.T) {
			t.Parallel()
			p1 := makeProfile()
			p2 := makeProfile()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("List", mock.Anything).Return([]ap.AgentProfile{p1, p2}, nil)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp AgentProfileListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Profiles, 2)
			assert.Equal(t, p1.Name, resp.Profiles[0].Name)
			assert.Equal(t, p2.Name, resp.Profiles[1].Name)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("List", mock.Anything).Return([]ap.AgentProfile(nil), errors.New("storage failure"))

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})

	t.Run("CreateAgentProfile", func(t *testing.T) {
		t.Parallel()

		t.Run("creates and returns 201", func(t *testing.T) {
			t.Parallel()
			profile := makeProfile()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Create", mock.Anything, ap.CreateAgentProfileParams{
				Name:         profile.Name,
				DisplayName:  profile.DisplayName,
				Role:         profile.Role,
				Instructions: profile.Instructions,
				ToolRefs:     profile.ToolRefs,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: profile.ExecutionSettings.DefaultModel,
				},
			}).Return(&profile, nil)

			h := newServerWithSvc(t, svc)
			body := `{"name":"` + profile.Name + `","displayName":"` + profile.DisplayName + `","role":"` +
				profile.Role + `","instructions":"` + profile.Instructions + `","toolRefs":["tool-a","tool-b"],` +
				`"executionSettings":{"defaultModel":"` + profile.ExecutionSettings.DefaultModel + `"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)
			var resp AgentProfileResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, profile.Name, resp.Name)
		})

		t.Run("returns 400 for malformed JSON", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(`{`),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("returns 400 for validation failure", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("role is required"))

			h := newServerWithSvc(t, svc)
			body := `{"name":"profile-a","role":"","instructions":"x","executionSettings":{"defaultModel":"openai/gpt-4.1"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("returns 409 for duplicate name", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Create", mock.Anything, mock.Anything).Return(nil, ap.ErrAgentProfileNameConflict)

			h := newServerWithSvc(t, svc)
			body := `{"name":"profile-a","role":"coder","instructions":"x","executionSettings":{"defaultModel":"openai/gpt-4.1"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusConflict, rec.Code)
		})
	})

	t.Run("GetAgentProfile", func(t *testing.T) {
		t.Parallel()

		t.Run("returns profile", func(t *testing.T) {
			t.Parallel()
			profile := makeProfile()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Get", mock.Anything, profile.Name).Return(&profile, nil)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/"+profile.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
		})

		t.Run("returns 404 for missing profile", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Get", mock.Anything, "missing").Return(nil, ap.ErrAgentProfileNotFound)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/missing", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Get", mock.Anything, "broken").Return(nil, errors.New("storage failure"))

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/broken", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})

	t.Run("UpdateAgentProfile", func(t *testing.T) {
		t.Parallel()

		t.Run("updates and returns 200", func(t *testing.T) {
			t.Parallel()
			profile := makeProfile()
			updated := profile
			updated.Role = "reviewer"
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Update", mock.Anything, profile.Name, ap.UpdateAgentProfileParams{
				DisplayName:  profile.DisplayName,
				Role:         updated.Role,
				Instructions: profile.Instructions,
				ToolRefs:     profile.ToolRefs,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: profile.ExecutionSettings.DefaultModel,
				},
			}).Return(&updated, nil)

			h := newServerWithSvc(t, svc)
			body := `{"displayName":"` + profile.DisplayName + `","role":"` + updated.Role + `","instructions":"` +
				profile.Instructions + `","toolRefs":["tool-a","tool-b"],"executionSettings":{"defaultModel":"` +
				profile.ExecutionSettings.DefaultModel + `"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/"+profile.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp AgentProfileResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, updated.Role, resp.Role)
		})

		t.Run("returns 400 for malformed JSON", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			h := newServerWithSvc(t, svc)

			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/profile-a",
				strings.NewReader(`{`),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("returns 400 for validation failure", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Update", mock.Anything, "profile-a", mock.Anything).Return(nil, errors.New("invalid payload"))

			h := newServerWithSvc(t, svc)
			body := `{"role":"","instructions":"x","executionSettings":{"defaultModel":"openai/gpt-4.1"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/profile-a",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusBadRequest, rec.Code)
		})

		t.Run("returns 404 for missing profile", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Update", mock.Anything, "missing", mock.Anything).Return(nil, ap.ErrAgentProfileNotFound)

			h := newServerWithSvc(t, svc)
			body := `{"role":"coder","instructions":"x","executionSettings":{"defaultModel":"openai/gpt-4.1"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/missing",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code)
		})
	})

	t.Run("DeleteAgentProfile", func(t *testing.T) {
		t.Parallel()

		t.Run("returns 204 on success", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Delete", mock.Anything, "profile-a").Return(nil)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/agent-profiles/profile-a", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNoContent, rec.Code)
		})

		t.Run("returns 404 for missing profile", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Delete", mock.Anything, "missing").Return(ap.ErrAgentProfileNotFound)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/agent-profiles/missing", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusNotFound, rec.Code)
		})

		t.Run("returns 500 on service error", func(t *testing.T) {
			t.Parallel()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Delete", mock.Anything, "broken").Return(errors.New("storage failure"))

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/agent-profiles/broken", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})
}
