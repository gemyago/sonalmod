//go:build !release

package agentapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	"github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSessionHandlers(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newHandler := func(t *testing.T, runner agent.AgentRunner) http.Handler {
		t.Helper()
		srv := NewAgentAPIServer(ServerParams{
			Runner:                 runner,
			Logger:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
			IDGen:                  NewMockIDGen(),
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
		})
		mux := http.NewServeMux()
		return HandlerFromMux(srv, mux)
	}

	t.Run("ListSessions", func(t *testing.T) {
		t.Parallel()

		t.Run("401 without caller identity", func(t *testing.T) {
			t.Parallel()

			m := agent.NewMockAgentRunner(t)
			h := newHandler(t, m)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/sessions?limit=10", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
		})

		t.Run("200 empty list", func(t *testing.T) {
			t.Parallel()

			userID := fake.Internet().User()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ListSessions(mock.Anything, mock.MatchedBy(func(p agent.ListSessionsParams) bool {
				return p.AppName == listSessionsAppName && p.UserID == userID && p.Limit == 10 && p.Offset == 0
			})).Return(&agent.ListSessionsResult{Sessions: nil, Total: 0}, nil)

			h := newHandler(t, m)
			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/sessions?limit=10", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp SessionListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Empty(t, resp.Sessions)
			assert.Equal(t, 0, resp.Total)
		})

		t.Run("200 with sessions and offset", func(t *testing.T) {
			t.Parallel()

			userID := fake.Internet().User()
			ts := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
			meta := rt.SessionMetadata{
				SessionID: fake.UUID().V4(),
				Title:     fake.Lorem().Word(),
				CreatedAt: ts,
				UpdatedAt: ts,
			}

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ListSessions(mock.Anything, mock.MatchedBy(func(p agent.ListSessionsParams) bool {
				return p.AppName == listSessionsAppName && p.UserID == userID && p.Limit == 5 && p.Offset == 2
			})).Return(&agent.ListSessionsResult{
				Sessions: []rt.SessionMetadata{meta},
				Total:    9,
			}, nil)

			h := newHandler(t, m)
			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/sessions?limit=5&offset=2", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp SessionListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Sessions, 1)
			assert.Equal(t, meta.SessionID, resp.Sessions[0].SessionId)
			assert.Equal(t, meta.Title, resp.Sessions[0].Title)
			assert.Equal(t, 9, resp.Total)
		})

		t.Run("200 applies title fallback when stored title empty", func(t *testing.T) {
			t.Parallel()

			userID := fake.Internet().User()
			ts := time.Unix(int64(fake.IntBetween(1577836800, 1893456000)), 0).UTC()
			wantTitle := "Session " + ts.Format("Jan 2 15:04")
			meta := rt.SessionMetadata{
				SessionID: fake.UUID().V4(),
				Title:     "",
				CreatedAt: ts,
				UpdatedAt: ts,
			}

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ListSessions(mock.Anything, mock.MatchedBy(func(p agent.ListSessionsParams) bool {
				return p.AppName == listSessionsAppName && p.UserID == userID && p.Limit == 10 && p.Offset == 0
			})).Return(&agent.ListSessionsResult{
				Sessions: []rt.SessionMetadata{meta},
				Total:    1,
			}, nil)

			h := newHandler(t, m)
			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/sessions?limit=10", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp SessionListResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Sessions, 1)
			assert.Equal(t, wantTitle, resp.Sessions[0].Title)
			assert.Equal(t, meta.SessionID, resp.Sessions[0].SessionId)
		})

		t.Run("500 on runner error", func(t *testing.T) {
			t.Parallel()

			userID := fake.Internet().User()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ListSessions(mock.Anything, mock.Anything).Return(nil, errors.New("store down"))

			h := newHandler(t, m)
			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/sessions?limit=10", nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	})
}
