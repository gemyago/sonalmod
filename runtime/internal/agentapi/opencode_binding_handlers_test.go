//go:build !release

package agentapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		bindingsSvc, err := agent.NewFileOpenCodeBindingService(t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
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
			strings.NewReader(`{"name":"bad binding","profileName":"profile-main","agentCommand":{"command":"","args":[]},"launchOptions":{"transport":"stdio"}}`),
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
}
