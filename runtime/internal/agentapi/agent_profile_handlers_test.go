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

	newFileProfilesService := func(t *testing.T) ap.AgentProfilesService {
		t.Helper()
		svc, err := ap.NewFileAgentProfilesService(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.NoError(t, err)
		return svc
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

	makeACPProfileWithCwd := func(cwd string) ap.AgentProfile {
		profile := ap.AgentProfile{
			Name:         "profile-" + fake.Lorem().Word(),
			DisplayName:  fake.Lorem().Word(),
			Role:         fake.Lorem().Word(),
			Instructions: fake.Lorem().Sentence(5),
			ToolRefs:     []string{"tool-a", "tool-b"},
			ExecutionSettings: ap.ExecutionSettings{
				Mode: ap.ExecutionModeACPStdio,
				AgentCommand: ap.ACPStdioAgentCommand{
					Command: "opencode",
					Args:    []string{"acp", "--safe"},
				},
			},
			CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Second),
			UpdatedAt: time.Now().UTC().Truncate(time.Second),
		}

		if cwd != "" {
			profile.ExecutionSettings.Cwd = cwd
		}

		return profile
	}

	makeACPProfile := func() ap.AgentProfile {
		return makeACPProfileWithCwd("/workspace")
	}

	createPersistedProfile := func(
		t *testing.T,
		svc ap.AgentProfilesService,
		profile ap.AgentProfile,
	) ap.AgentProfile {
		t.Helper()

		created, err := svc.Create(t.Context(), ap.CreateAgentProfileParams{
			Name:              profile.Name,
			DisplayName:       profile.DisplayName,
			Role:              profile.Role,
			Instructions:      profile.Instructions,
			ToolRefs:          profile.ToolRefs,
			ExecutionSettings: profile.ExecutionSettings,
		})
		require.NoError(t, err)

		return *created
	}

	mustJSON := func(t *testing.T, value any) string {
		t.Helper()
		payload, err := json.Marshal(value)
		require.NoError(t, err)
		return string(payload)
	}

	makeRegularExecutionSettings := func(
		t *testing.T,
		defaultModel string,
		includeMode bool,
	) AgentProfileExecutionSettings {
		t.Helper()

		settings := AgentProfileRegularExecutionSettings{
			DefaultModel: defaultModel,
		}
		if includeMode {
			mode := Regular
			settings.Mode = &mode
		}

		var union AgentProfileExecutionSettings
		require.NoError(t, union.FromAgentProfileRegularExecutionSettings(settings))
		return union
	}

	makeACPExecutionSettings := func(
		t *testing.T,
		command string,
		args []string,
		cwd string,
	) AgentProfileExecutionSettings {
		t.Helper()

		settings := AgentProfileACPStdioExecutionSettings{
			Mode: "acp-stdio",
			AgentCommand: ACPStdioAgentCommand{
				Command: command,
			},
		}
		if len(args) > 0 {
			settings.AgentCommand.Args = &args
		}
		if cwd != "" {
			settings.Cwd = &cwd
		}

		var union AgentProfileExecutionSettings
		require.NoError(t, union.FromAgentProfileACPStdioExecutionSettings(settings))
		return union
	}

	decodeProfileResponse := func(t *testing.T, body io.Reader) AgentProfileResponse {
		t.Helper()

		var resp AgentProfileResponse
		require.NoError(t, json.NewDecoder(body).Decode(&resp))
		return resp
	}

	assertRegularExecutionSettings := func(
		t *testing.T,
		settings AgentProfileExecutionSettings,
		expectedMode *AgentProfileRegularExecutionSettingsMode,
		expectedDefaultModel string,
	) {
		t.Helper()

		regularSettings, err := settings.AsAgentProfileRegularExecutionSettings()
		require.NoError(t, err)
		assert.Equal(t, expectedDefaultModel, regularSettings.DefaultModel)
		assert.Equal(t, expectedMode, regularSettings.Mode)
	}

	assertACPExecutionSettings := func(
		t *testing.T,
		settings AgentProfileExecutionSettings,
		expectedCommand string,
		expectedArgs []string,
		expectedCwd *string,
	) {
		t.Helper()

		acpSettings, err := settings.AsAgentProfileACPStdioExecutionSettings()
		require.NoError(t, err)
		assert.Equal(t, "acp-stdio", acpSettings.Mode)
		assert.Equal(t, expectedCommand, acpSettings.AgentCommand.Command)
		assert.Equal(t, expectedArgs, stringSliceValue(acpSettings.AgentCommand.Args))
		assert.Equal(t, expectedCwd, acpSettings.Cwd)
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

		t.Run("returns persisted legacy regular and acp-stdio execution settings", func(t *testing.T) {
			t.Parallel()

			svc := newFileProfilesService(t)
			regularProfile := createPersistedProfile(t, svc, makeProfile())
			acpProfile := createPersistedProfile(t, svc, makeACPProfile())

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)

			var resp AgentProfileListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Profiles, 2)

			profilesByName := make(map[string]AgentProfileResponse, len(resp.Profiles))
			for _, profile := range resp.Profiles {
				profilesByName[profile.Name] = profile
			}

			regularResp, ok := profilesByName[regularProfile.Name]
			require.True(t, ok)
			assertRegularExecutionSettings(
				t,
				regularResp.ExecutionSettings,
				nil,
				regularProfile.ExecutionSettings.DefaultModel,
			)

			acpResp, ok := profilesByName[acpProfile.Name]
			require.True(t, ok)
			cwd := "/workspace"
			assertACPExecutionSettings(
				t,
				acpResp.ExecutionSettings,
				"opencode",
				[]string{"acp", "--safe"},
				&cwd,
			)
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
			body := mustJSON(t, CreateAgentProfileRequest{
				Name:         profile.Name,
				DisplayName:  &profile.DisplayName,
				Role:         profile.Role,
				Instructions: profile.Instructions,
				ToolRefs:     &profile.ToolRefs,
				ExecutionSettings: makeRegularExecutionSettings(
					t,
					profile.ExecutionSettings.DefaultModel,
					false,
				),
			})
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			assert.Equal(t, profile.Name, resp.Name)
			assertRegularExecutionSettings(t, resp.ExecutionSettings, nil, profile.ExecutionSettings.DefaultModel)
		})

		t.Run("accepts acp-stdio execution settings without default model", func(t *testing.T) {
			t.Parallel()
			profile := makeACPProfile()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Create", mock.Anything, ap.CreateAgentProfileParams{
				Name:         profile.Name,
				DisplayName:  profile.DisplayName,
				Role:         profile.Role,
				Instructions: profile.Instructions,
				ToolRefs:     profile.ToolRefs,
				ExecutionSettings: ap.ExecutionSettings{
					Mode: ap.ExecutionModeACPStdio,
					AgentCommand: ap.ACPStdioAgentCommand{
						Command: "opencode",
						Args:    []string{"acp", "--safe"},
					},
					Cwd: "/workspace",
				},
			}).Return(&profile, nil)

			h := newServerWithSvc(t, svc)
			body := mustJSON(t, CreateAgentProfileRequest{
				Name:         profile.Name,
				DisplayName:  &profile.DisplayName,
				Role:         profile.Role,
				Instructions: profile.Instructions,
				ToolRefs:     &profile.ToolRefs,
				ExecutionSettings: makeACPExecutionSettings(
					t,
					"opencode",
					[]string{"acp", "--safe"},
					"/workspace",
				),
			})
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusCreated, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			cwd := "/workspace"
			assertACPExecutionSettings(t, resp.ExecutionSettings, "opencode", []string{"acp", "--safe"}, &cwd)
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

		t.Run("returns 400 for unsupported execution mode", func(t *testing.T) {
			t.Parallel()
			h := newServerWithSvc(t, newFileProfilesService(t))
			body := `{"name":"profile-a","role":"coder","instructions":"x","executionSettings":{"mode":"remote","defaultModel":"openai/gpt-4.1"}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "execution_settings.mode")
		})

		t.Run("returns 400 for invalid acp command settings", func(t *testing.T) {
			t.Parallel()
			h := newServerWithSvc(t, newFileProfilesService(t))
			body := `{"name":"profile-a","role":"coder","instructions":"x","executionSettings":{"mode":"acp-stdio","agentCommand":{"command":"opencode","args":["acp"," acp "]}}}`
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"/agent-profiles",
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "execution_settings.agent_command.args")
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
			resp := decodeProfileResponse(t, rec.Body)
			assertRegularExecutionSettings(t, resp.ExecutionSettings, nil, profile.ExecutionSettings.DefaultModel)
		})

		t.Run("returns persisted legacy regular profile without mode", func(t *testing.T) {
			t.Parallel()

			svc := newFileProfilesService(t)
			profile := createPersistedProfile(t, svc, makeProfile())

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/"+profile.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			assertRegularExecutionSettings(t, resp.ExecutionSettings, nil, profile.ExecutionSettings.DefaultModel)
		})

		t.Run("returns acp-stdio execution settings", func(t *testing.T) {
			t.Parallel()
			profile := makeACPProfile()
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Get", mock.Anything, profile.Name).Return(&profile, nil)

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/"+profile.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			cwd := "/workspace"
			assertACPExecutionSettings(t, resp.ExecutionSettings, "opencode", []string{"acp", "--safe"}, &cwd)
		})

		t.Run("returns persisted acp-stdio execution settings without cwd", func(t *testing.T) {
			t.Parallel()

			svc := newFileProfilesService(t)
			profile := createPersistedProfile(t, svc, makeACPProfileWithCwd(""))

			h := newServerWithSvc(t, svc)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/agent-profiles/"+profile.Name, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			assertACPExecutionSettings(t, resp.ExecutionSettings, "opencode", []string{"acp", "--safe"}, nil)
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
			body := mustJSON(t, UpdateAgentProfileRequest{
				DisplayName:  &profile.DisplayName,
				Role:         updated.Role,
				Instructions: profile.Instructions,
				ToolRefs:     &profile.ToolRefs,
				ExecutionSettings: makeRegularExecutionSettings(
					t,
					profile.ExecutionSettings.DefaultModel,
					false,
				),
			})
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/"+profile.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			assert.Equal(t, updated.Role, resp.Role)
			assertRegularExecutionSettings(t, resp.ExecutionSettings, nil, profile.ExecutionSettings.DefaultModel)
		})

		t.Run("accepts acp-stdio execution settings without default model", func(t *testing.T) {
			t.Parallel()
			profile := makeProfile()
			updated := makeACPProfile()
			updated.Name = profile.Name
			updated.DisplayName = profile.DisplayName
			svc := &mockAgentProfilesService{}
			svc.Test(t)
			svc.On("Update", mock.Anything, profile.Name, ap.UpdateAgentProfileParams{
				DisplayName:  profile.DisplayName,
				Role:         updated.Role,
				Instructions: updated.Instructions,
				ToolRefs:     updated.ToolRefs,
				ExecutionSettings: ap.ExecutionSettings{
					Mode: ap.ExecutionModeACPStdio,
					AgentCommand: ap.ACPStdioAgentCommand{
						Command: "opencode",
						Args:    []string{"acp", "--safe"},
					},
					Cwd: "/workspace",
				},
			}).Return(&updated, nil)

			h := newServerWithSvc(t, svc)
			body := mustJSON(t, UpdateAgentProfileRequest{
				DisplayName:  &profile.DisplayName,
				Role:         updated.Role,
				Instructions: updated.Instructions,
				ToolRefs:     &updated.ToolRefs,
				ExecutionSettings: makeACPExecutionSettings(
					t,
					"opencode",
					[]string{"acp", "--safe"},
					"/workspace",
				),
			})
			req := httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPut,
				"/agent-profiles/"+profile.Name,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			resp := decodeProfileResponse(t, rec.Body)
			cwd := "/workspace"
			assertACPExecutionSettings(t, resp.ExecutionSettings, "opencode", []string{"acp", "--safe"}, &cwd)
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

func stringSliceValue(value *[]string) []string {
	if value == nil {
		return nil
	}

	return append([]string(nil), (*value)...)
}
