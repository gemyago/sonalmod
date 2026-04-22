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
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeLaunchHandlers(t *testing.T) {
	t.Parallel()

	newHandler := func(t *testing.T) http.Handler {
		t.Helper()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 agent.NewMockAgentRunner(t),
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
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

		var payload map[string]any
		require.NoError(t, json.NewDecoder(res.Body).Decode(&payload))
		assert.NotEmpty(t, payload["sessionId"])
	})

	t.Run("maps launch error to problem details", func(t *testing.T) {
		t.Parallel()
		h := newHandler(t)
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
}
