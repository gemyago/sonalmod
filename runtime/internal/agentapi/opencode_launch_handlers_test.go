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

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeLaunchHandlers(t *testing.T) {
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
			OpenCodeLauncher: newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{
				ProfileName: "profile-main",
				BindingName: "binding-main",
				SessionID:   "session-main",
			}),
		})
		return HandlerFromMux(srv, http.NewServeMux())
	}

	t.Run("requires authentication", func(t *testing.T) {
		t.Parallel()
		h := newHandler(t)
		body := `{"profileName":"profile-main","bindingName":"binding-main","prompt":"Write tests"}`
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(body),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("accepts selector and prompt and returns launch payload", func(t *testing.T) {
		t.Parallel()
		h := newHandler(t)
		ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: "user-main"})
		body := `{"profileName":"profile-main","bindingName":"binding-main","prompt":"Write tests"}`
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(body),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusCreated, res.Code)

		var payload OpenCodeLaunchResponse
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		assert.Equal(t, "session-main", payload.SessionId)
		assert.Equal(t, "profile-main", payload.ProfileName)
		assert.Equal(t, "binding-main", payload.BindingName)
	})

	t.Run("maps validation launch error to bad request", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithLaunchError(t, &agent.OpenCodeLaunchError{
			Kind: agent.OpenCodeLaunchErrorKindValidation,
			Op:   "validate-prompt",
			Err:  errors.New("prompt is required"),
		})
		ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: "user-main"})
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","prompt":"bad"}`),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Code)
	})

	t.Run("maps launch error to problem details", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithLaunchError(t, &agent.OpenCodeLaunchError{
			Kind: agent.OpenCodeLaunchErrorKindNotFound,
			Op:   "resolve-binding",
			Err:  errors.New("missing"),
		})
		ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: "user-main"})
		body := `{"profileName":"missing","prompt":"Write tests"}`
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(body),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusNotFound, res.Code)

		var pd ProblemDetails
		require.NoError(t, json.NewDecoder(res.Body).Decode(&pd))
		require.NotNil(t, pd.Status)
		assert.Equal(t, http.StatusNotFound, *pd.Status)
	})

	t.Run("maps launch-failed errors to internal server error", func(t *testing.T) {
		t.Parallel()
		h := newHandlerWithLaunchError(t, &agent.OpenCodeLaunchError{
			Kind: agent.OpenCodeLaunchErrorKindLaunchFailed,
			Op:   "launch-acp-session",
			Err:  errors.New("subprocess failed"),
		})
		ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: "user-main"})
		req := httptest.NewRequestWithContext(
			ctx,
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","prompt":"Write tests"}`),
		)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		require.Equal(t, http.StatusInternalServerError, res.Code)
	})
}

func newHandlerWithLaunchError(t *testing.T, launchErr error) http.Handler {
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
		OpenCodeLauncher:       newStubOpenCodeLauncherWithError(launchErr),
	})
	return HandlerFromMux(srv, http.NewServeMux())
}

type stubOpenCodeLauncher struct {
	result cl.OpenCodeLaunchResult
	err    error
}

func newStubOpenCodeLauncher(result cl.OpenCodeLaunchResult) *stubOpenCodeLauncher {
	return &stubOpenCodeLauncher{result: result}
}

func newStubOpenCodeLauncherWithError(err error) *stubOpenCodeLauncher {
	return &stubOpenCodeLauncher{err: err}
}

func (s *stubOpenCodeLauncher) Launch(
	_ context.Context,
	_ agent.OpenCodeLaunchRequest,
) (*agent.OpenCodeLaunchResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	copied := s.result
	return &copied, nil
}
