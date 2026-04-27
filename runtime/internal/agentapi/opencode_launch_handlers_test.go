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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeLaunchHandlers(t *testing.T) {
	t.Parallel()

	newLogger := func() *slog.Logger {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	newServer := func(launcher agent.OpenCodeLauncher) *AgentAPIServer {
		return NewAgentAPIServer(ServerParams{
			Logger:           newLogger(),
			OpenCodeLauncher: launcher,
		})
	}
	newAuthContext := func(t *testing.T) context.Context {
		t.Helper()
		return callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: "user-main"})
	}

	t.Run("launch route is not exposed", func(t *testing.T) {
		t.Parallel()

		h := HandlerFromMux(newServer(nil), http.NewServeMux())
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{}`),
		)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("direct create handler requires authentication", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}))
		req := httptest.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","bindingName":"binding-main","prompt":"Write tests"}`),
		)
		rec := httptest.NewRecorder()

		srv.CreateOpenCodeLaunch(rec, req)

		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("direct create handler returns success payload", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{
			ProfileName: "profile-main",
			BindingName: "binding-main",
			SessionID:   "session-main",
		}))
		req := httptest.NewRequestWithContext(
			newAuthContext(t),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","bindingName":"binding-main","prompt":"Write tests"}`),
		)
		rec := httptest.NewRecorder()

		srv.CreateOpenCodeLaunch(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)

		var payload OpenCodeLaunchResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
		assert.Equal(t, "session-main", payload.SessionId)
		assert.Equal(t, "profile-main", payload.ProfileName)
		assert.Equal(t, "binding-main", payload.BindingName)
	})

	t.Run("direct create handler maps request validation failures", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{}))
		authCtx := newAuthContext(t)
		testCases := []struct {
			name string
			body string
		}{
			{name: "malformed", body: `{`},
			{name: "missing profile", body: `{"profileName":" ","prompt":"hello"}`},
			{name: "missing prompt", body: `{"profileName":"profile-main","prompt":" "}`},
		}

		for _, tc := range testCases {
			req := httptest.NewRequestWithContext(
				authCtx,
				http.MethodPost,
				"/opencode-launches",
				strings.NewReader(tc.body),
			)
			rec := httptest.NewRecorder()

			srv.CreateOpenCodeLaunch(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code, tc.name)
		}
	})

	t.Run("direct create handler maps launcher errors", func(t *testing.T) {
		t.Parallel()

		authCtx := newAuthContext(t)
		testCases := []struct {
			name   string
			err    error
			status int
		}{
			{
				name: "validation",
				err: &agent.OpenCodeLaunchError{
					Kind: agent.OpenCodeLaunchErrorKindValidation,
					Op:   "validate-prompt",
					Err:  errors.New("prompt is required"),
				},
				status: http.StatusBadRequest,
			},
			{
				name: "not found",
				err: &agent.OpenCodeLaunchError{
					Kind: agent.OpenCodeLaunchErrorKindNotFound,
					Op:   "resolve-binding",
					Err:  errors.New("missing"),
				},
				status: http.StatusNotFound,
			},
			{
				name: "launch failed",
				err: &agent.OpenCodeLaunchError{
					Kind: agent.OpenCodeLaunchErrorKindLaunchFailed,
					Op:   "launch-acp-session",
					Err:  errors.New("subprocess failed"),
				},
				status: http.StatusInternalServerError,
			},
			{
				name:   "unknown",
				err:    errors.New("unknown failure"),
				status: http.StatusInternalServerError,
			},
		}

		for _, tc := range testCases {
			srv := newServer(newStubOpenCodeLauncherWithError(tc.err))
			req := httptest.NewRequestWithContext(
				authCtx,
				http.MethodPost,
				"/opencode-launches",
				strings.NewReader(`{"profileName":"profile-main","prompt":"Write tests"}`),
			)
			rec := httptest.NewRecorder()

			srv.CreateOpenCodeLaunch(rec, req)

			require.Equal(t, tc.status, rec.Code, tc.name)
		}
	})

	t.Run("direct create handler returns internal error when launcher is missing", func(t *testing.T) {
		t.Parallel()

		srv := newServer(nil)
		req := httptest.NewRequestWithContext(
			newAuthContext(t),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","prompt":"Write tests"}`),
		)
		rec := httptest.NewRecorder()

		srv.CreateOpenCodeLaunch(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("response mapper tolerates non-object payloads", func(t *testing.T) {
		t.Parallel()

		srv := newServer(newStubOpenCodeLauncher(cl.OpenCodeLaunchResult{
			ProfileName:  "profile-main",
			BindingName:  "binding-main",
			SessionID:    "session-main",
			PromptResult: []byte(`"text"`),
			Updates: []cl.OpenCodeACPUpdate{
				{SessionID: "session-main", Type: "delta", Payload: []byte(`[]`)},
			},
		}))
		req := httptest.NewRequestWithContext(
			newAuthContext(t),
			http.MethodPost,
			"/opencode-launches",
			strings.NewReader(`{"profileName":"profile-main","prompt":"Write tests"}`),
		)
		rec := httptest.NewRecorder()

		srv.CreateOpenCodeLaunch(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)

		var payload OpenCodeLaunchResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&payload))
		assert.Equal(t, map[string]any{}, payload.PromptResult)
		require.Len(t, payload.Updates, 1)
		assert.Equal(t, map[string]any{}, payload.Updates[0].Payload)
	})
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
	return &agent.OpenCodeLaunchResult{
		ProfileName:  s.result.ProfileName,
		BindingName:  s.result.BindingName,
		SessionID:    s.result.SessionID,
		PromptResult: append([]byte(nil), s.result.PromptResult...),
		Updates:      append([]cl.OpenCodeACPUpdate(nil), s.result.Updates...),
	}, nil
}
