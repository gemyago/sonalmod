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
	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBindingHandlers(t *testing.T) {
	t.Parallel()

	newHandler := func(t *testing.T) http.Handler {
		t.Helper()
		bindingsSvc, err := agent.NewFileOpenCodeBindingService(
			t.TempDir(),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)
		require.NoError(t, err)
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
			OpenCodeBindingService: bindingsSvc,
			OpenCodeLauncher:       newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}),
		})
		return HandlerFromMux(srv, http.NewServeMux())
	}

	t.Run("creates then gets then deletes binding", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t)
		createBody := `{"name":"binding-main","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`
		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(createBody),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusCreated, createRes.Code)

		getReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings/binding-main",
			nil,
		)
		getRes := httptest.NewRecorder()
		h.ServeHTTP(getRes, getReq)
		require.Equal(t, http.StatusOK, getRes.Code)

		deleteReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodDelete,
			"/opencode-bindings/binding-main",
			nil,
		)
		deleteRes := httptest.NewRecorder()
		h.ServeHTTP(deleteRes, deleteReq)
		require.Equal(t, http.StatusNoContent, deleteRes.Code)
	})

	t.Run("returns conflict and not-found problem details", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t)
		createBody := `{"name":"binding-conflict","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`

		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(createBody),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusCreated, createRes.Code)

		createAgainReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(createBody),
		)
		createAgainRes := httptest.NewRecorder()
		h.ServeHTTP(createAgainRes, createAgainReq)
		require.Equal(t, http.StatusConflict, createAgainRes.Code)

		missingReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodGet,
			"/opencode-bindings/not-found",
			nil,
		)
		missingRes := httptest.NewRecorder()
		h.ServeHTTP(missingRes, missingReq)
		require.Equal(t, http.StatusNotFound, missingRes.Code)

		var pd ProblemDetails
		require.NoError(t, json.NewDecoder(missingRes.Body).Decode(&pd))
		require.NotNil(t, pd.Status)
		assert.Equal(t, http.StatusNotFound, *pd.Status)
	})

	t.Run("returns bad request for invalid payload", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t)
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(
				`{"name":"bad binding","profileName":"profile-main","agentCommand":{"command":"","args":[]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Code)
	})

	t.Run("updates binding", func(t *testing.T) {
		t.Parallel()

		h := newHandler(t)
		createBody := `{"name":"binding-update","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`
		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(createBody),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusCreated, createRes.Code)

		updateBody := `{"cwd":"/tmp/new","agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`
		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-update",
			strings.NewReader(updateBody),
		)
		updateRes := httptest.NewRecorder()
		h.ServeHTTP(updateRes, updateReq)
		require.Equal(t, http.StatusOK, updateRes.Code)

		var updated OpenCodeBindingResponse
		require.NoError(t, json.NewDecoder(updateRes.Body).Decode(&updated))
		assert.Equal(t, "/tmp/new", updated.Cwd)
		assert.Equal(t, []string{"--safe"}, updated.AgentCommand.Args)
	})

	t.Run("lists bindings", func(t *testing.T) {
		t.Parallel()
		h := newHandler(t)

		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(
				`{"name":"binding-list","profileName":"profile-main","cwd":"/tmp","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusCreated, createRes.Code)

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/opencode-bindings", nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code)

		var payload OpenCodeBindingListResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		require.Len(t, payload.Bindings, 1)
	})

	t.Run("returns internal error when service is missing", func(t *testing.T) {
		t.Parallel()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
			OpenCodeLauncher:       newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}),
		})
		h := HandlerFromMux(srv, http.NewServeMux())

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/opencode-bindings", nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusInternalServerError, res.Code)
	})

	t.Run("returns internal error on list failure", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithBindingService(t, &stubOpenCodeBindingService{
			listFn: func(context.Context) ([]agent.OpenCodeBinding, error) {
				return nil, errors.New("boom")
			},
		})

		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/opencode-bindings", nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusInternalServerError, res.Code)
	})

	t.Run("returns internal error on get and delete failures", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithBindingService(t, &stubOpenCodeBindingService{
			getFn: func(context.Context, string) (*agent.OpenCodeBinding, error) {
				return nil, errors.New("boom")
			},
			deleteFn: func(context.Context, string) error {
				return errors.New("boom")
			},
		})

		getReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/opencode-bindings/fail", nil)
		getRes := httptest.NewRecorder()
		h.ServeHTTP(getRes, getReq)
		require.Equal(t, http.StatusInternalServerError, getRes.Code)

		deleteReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/opencode-bindings/fail", nil)
		deleteRes := httptest.NewRecorder()
		h.ServeHTTP(deleteRes, deleteReq)
		require.Equal(t, http.StatusInternalServerError, deleteRes.Code)
	})

	t.Run("returns not found on update and delete missing bindings", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithBindingService(t, &stubOpenCodeBindingService{
			updateFn: func(context.Context, string, agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error) {
				return nil, agent.ErrOpenCodeBindingNotFound
			},
			deleteFn: func(context.Context, string) error {
				return agent.ErrOpenCodeBindingNotFound
			},
		})

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/missing",
			strings.NewReader(
				`{"cwd":"/tmp","agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRes := httptest.NewRecorder()
		h.ServeHTTP(updateRes, updateReq)
		require.Equal(t, http.StatusNotFound, updateRes.Code)

		deleteReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/opencode-bindings/missing", nil)
		deleteRes := httptest.NewRecorder()
		h.ServeHTTP(deleteRes, deleteReq)
		require.Equal(t, http.StatusNotFound, deleteRes.Code)
	})

	t.Run("returns internal error when service missing for all CRUD operations", func(t *testing.T) {
		t.Parallel()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
			OpenCodeLauncher:       newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}),
		})
		h := HandlerFromMux(srv, http.NewServeMux())

		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(
				`{"name":"binding-a","profileName":"profile-main","agentCommand":{"command":"opencode","args":["--fast"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusInternalServerError, createRes.Code)

		getReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/opencode-bindings/binding-a", nil)
		getRes := httptest.NewRecorder()
		h.ServeHTTP(getRes, getReq)
		require.Equal(t, http.StatusInternalServerError, getRes.Code)

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-a",
			strings.NewReader(
				`{"agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRes := httptest.NewRecorder()
		h.ServeHTTP(updateRes, updateReq)
		require.Equal(t, http.StatusInternalServerError, updateRes.Code)

		deleteReq := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/opencode-bindings/binding-a", nil)
		deleteRes := httptest.NewRecorder()
		h.ServeHTTP(deleteRes, deleteReq)
		require.Equal(t, http.StatusInternalServerError, deleteRes.Code)
	})

	t.Run("returns bad request for malformed create and update payloads", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithBindingService(t, &stubOpenCodeBindingService{})

		createReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-bindings",
			strings.NewReader(`{`),
		)
		createRes := httptest.NewRecorder()
		h.ServeHTTP(createRes, createReq)
		require.Equal(t, http.StatusBadRequest, createRes.Code)

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-a",
			strings.NewReader(`{`),
		)
		updateRes := httptest.NewRecorder()
		h.ServeHTTP(updateRes, updateReq)
		require.Equal(t, http.StatusBadRequest, updateRes.Code)
	})

	t.Run("returns bad request on update validation errors", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithBindingService(t, &stubOpenCodeBindingService{
			updateFn: func(context.Context, string, agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error) {
				return nil, errors.New("bad update payload")
			},
		})

		updateReq := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPut,
			"/opencode-bindings/binding-a",
			strings.NewReader(
				`{"agentCommand":{"command":"opencode","args":["--safe"]},"launchOptions":{"transport":"stdio"}}`,
			),
		)
		updateRes := httptest.NewRecorder()
		h.ServeHTTP(updateRes, updateReq)
		require.Equal(t, http.StatusBadRequest, updateRes.Code)
	})
}

func newHandlerWithBindingService(t *testing.T, bindings agent.OpenCodeBindingService) http.Handler {
	t.Helper()
	srv := NewAgentAPIServer(ServerParams{
		Runner:                 agent.NewMockAgentRunner(t),
		Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		IDGen:                  NewMockIDGen(),
		RequestMapper:          NewAgentAPIRequestMapper(),
		SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
		ProvidersConfigService: lp.NewMockProvidersConfigService(t),
		AgentProfilesService:   &mockAgentProfilesService{},
		OpenCodeBindingService: bindings,
		OpenCodeLauncher:       newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}),
	})
	return HandlerFromMux(srv, http.NewServeMux())
}

type stubOpenCodeBindingService struct {
	listFn   func(ctx context.Context) ([]agent.OpenCodeBinding, error)
	getFn    func(ctx context.Context, name string) (*agent.OpenCodeBinding, error)
	createFn func(ctx context.Context, params agent.CreateOpenCodeBindingParams) (*agent.OpenCodeBinding, error)
	updateFn func(ctx context.Context, name string, params agent.UpdateOpenCodeBindingParams) (*agent.OpenCodeBinding, error)
	deleteFn func(ctx context.Context, name string) error
}

func (s *stubOpenCodeBindingService) List(ctx context.Context) ([]agent.OpenCodeBinding, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return []agent.OpenCodeBinding{}, nil
}

func (s *stubOpenCodeBindingService) Get(ctx context.Context, name string) (*agent.OpenCodeBinding, error) {
	if s.getFn != nil {
		return s.getFn(ctx, name)
	}
	return &agent.OpenCodeBinding{
		Name:        name,
		ProfileName: "profile-main",
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"--fast"},
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{Transport: "stdio"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}, nil
}

func (s *stubOpenCodeBindingService) Create(
	ctx context.Context,
	params agent.CreateOpenCodeBindingParams,
) (*agent.OpenCodeBinding, error) {
	if s.createFn != nil {
		return s.createFn(ctx, params)
	}
	now := time.Now().UTC()
	return &agent.OpenCodeBinding{
		Name:          params.Name,
		ProfileName:   params.ProfileName,
		CWD:           params.CWD,
		AgentCommand:  params.AgentCommand,
		LaunchOptions: params.LaunchOptions,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (s *stubOpenCodeBindingService) Update(
	ctx context.Context,
	name string,
	params agent.UpdateOpenCodeBindingParams,
) (*agent.OpenCodeBinding, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, name, params)
	}
	now := time.Now().UTC()
	return &agent.OpenCodeBinding{
		Name:        name,
		ProfileName: "profile-main",
		CWD:         params.CWD,
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: params.AgentCommand.Command,
			Args:    append([]string(nil), params.AgentCommand.Args...),
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{Transport: params.LaunchOptions.Transport},
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (s *stubOpenCodeBindingService) Delete(ctx context.Context, name string) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, name)
	}
	return nil
}

func (s *stubOpenCodeBindingService) AutoMigrate() error {
	return nil
}
