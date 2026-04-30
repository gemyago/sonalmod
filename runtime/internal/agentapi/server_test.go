//go:build !release

package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	"github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// fakeCallerIdentity is a test implementation of callerid.Identity.
type fakeCallerIdentity struct{ userID string }

func (f *fakeCallerIdentity) UserID() string { return f.userID }

//nolint:gocyclo,cyclop // integration-style handler coverage keeps scenarios together.
func TestAgentAPIServer(t *testing.T) {
	newTestAgentAPIServerWithProfiles := func(
		t *testing.T,
		runner agent.AgentRunner,
		gen IDGen,
		profilesSvc *mockAgentProfilesService,
	) *AgentAPIServer {
		t.Helper()
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		if profilesSvc == nil {
			profilesSvc = &mockAgentProfilesService{}
		}

		return NewAgentAPIServer(ServerParams{
			Runner:                 runner,
			Logger:                 log,
			IDGen:                  gen,
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
			AgentProfilesService:   profilesSvc,
		})
	}
	newTestAgentAPIServer := func(t *testing.T, runner agent.AgentRunner, gen IDGen) *AgentAPIServer {
		t.Helper()
		return newTestAgentAPIServerWithProfiles(t, runner, gen, nil)
	}

	t.Run("StartAgentRun", func(t *testing.T) {
		makeReq := func(t *testing.T, ctx context.Context, msg, profileName, path string) *http.Request {
			t.Helper()
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			if profileName != "" {
				body = fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			}
			return httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Message != nil &&
					p.Model == "" &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)

			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

			blocks := parseSSEBlocks(rec.Body.String())
			require.GreaterOrEqual(t, len(blocks), 2)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "done", blocks[len(blocks)-1].event)

			var sb SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sb))
			assert.Equal(t, expSID, sb.SessionId)
		})

		t.Run("success_SSE_with_agent_event", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			chunk := fake.Lorem().Word()
			profileName := "profile-" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: chunk}}}
			ev.Partial = true

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Model == "" &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(expSID, []*session.Event{ev}), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "agent", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)
		})

		t.Run("malformedJSON_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(`{`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusBadRequest, *pd.Status)
		})

		t.Run("callerIdentityAbsent_401", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			msg := fake.Lorem().Sentence(3)

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			req := makeReq(t, t.Context(), msg, "", "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusUnauthorized, *pd.Status)
		})

		t.Run("invalidMessage_emptyParts_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := `{"message":{"parts":[]}}`
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "message parts")
		})

		t.Run("runnerError_500", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			runErr := errors.New(fake.Lorem().Sentence(4))
			profileName := "profile-" + fake.Lorem().Word()

			gen := NewMockIDGen()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent run failed")
		})

		t.Run("nilRunResult_logsStreamError", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			gen := NewMockIDGen()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// StreamAgentRun(nil) returns before writing SSE; handler only logs the error.
			assert.Empty(t, rec.Header().Get("Content-Type"))
		})

		t.Run("missing_profile_and_model_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "model is required when profileName is not provided")
		})

		t.Run("missing_profileName_uses_request_model", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			modelName := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Message != nil &&
					p.Model == modelName
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"model":%q,"message":{"parts":[{"text":%q}]}}`, modelName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("profileName_dispatches_regular_profile_default_model", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Model == "" &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(
				`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`,
				profileName,
				msg,
			)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("blank_profileName_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":"   ","message":{"parts":[{"text":%q}]}}`, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "profileName must not be blank")
		})

		t.Run("profileName_request_model_overrides_regular_profile_default", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			overrideModel := "myprovider/" + fake.Lorem().Word()
			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Model == overrideModel &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(
				`{"profileName":%q,"model":%q,"message":{"parts":[{"text":%q}]}}`,
				profileName,
				overrideModel,
				msg,
			)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("unknown_profileName_returns_404", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, &rt.AgentExecError{
				Kind: rt.AgentExecErrorKindNotFound,
				Op:   "load-profile",
				Err:  ap.ErrAgentProfileNotFound,
			})
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent profile not found")
		})
	})

	t.Run("ReadSession", func(t *testing.T) {
		sessionPath := func(sessionID string) string {
			return "/sessions/" + sessionID
		}

		makeIdleOutput := func(sessionID string, events []*rt.SessionEvent) *rt.ReadSessionResult {
			seq := func(yield func(*rt.SessionEvent, error) bool) {
				for _, e := range events {
					if !yield(e, nil) {
						return
					}
				}
			}
			return rt.NewReadSessionResult(sessionID, false, seq)
		}

		t.Run("idleSession_replaysHistoryAndDone", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessID := fake.UUID().V4()
			userID := fake.Internet().User()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ReadSession(mock.Anything, agent.ReadSessionParams{
				SessionID: sessID,
				UserID:    userID,
			}).Return(makeIdleOutput(sessID, nil), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, sessionPath(sessID), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

			blocks := parseSSEBlocks(rec.Body.String())
			require.GreaterOrEqual(t, len(blocks), 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "sessionStatus", blocks[1].event)
			assert.Equal(t, "done", blocks[len(blocks)-1].event)

			var sb SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sb))
			assert.Equal(t, sessID, sb.SessionId)

			var ss SessionStatusEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[1].data), &ss))
			assert.Equal(t, Idle, ss.Status)
		})

		t.Run("idleSession_withHistory_replaysEventsAndDone", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessID := fake.UUID().V4()
			userID := fake.Internet().User()

			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: fake.Lorem().Word()}}}
			sessEv := rt.MapADKSessionEvent(ev)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ReadSession(mock.Anything, agent.ReadSessionParams{
				SessionID: sessID,
				UserID:    userID,
			}).Return(makeIdleOutput(sessID, []*rt.SessionEvent{sessEv}), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, sessionPath(sessID), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 4) // sessionBound, sessionStatus, agent, done
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "sessionStatus", blocks[1].event)
			assert.Equal(t, "agent", blocks[2].event)
			assert.Equal(t, "done", blocks[3].event)
		})

		t.Run("activeSession_replaysHistoryThenLive", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessID := fake.UUID().V4()
			userID := fake.Internet().User()

			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: fake.Lorem().Word()}}}
			sessEv := rt.MapADKSessionEvent(ev)

			seq := func(yield func(*rt.SessionEvent, error) bool) {
				yield(sessEv, nil)
			}
			output := rt.NewReadSessionResult(sessID, true, seq)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ReadSession(mock.Anything, agent.ReadSessionParams{
				SessionID: sessID,
				UserID:    userID,
			}).Return(output, nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, sessionPath(sessID), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 4) // sessionBound, sessionStatus(active), agent, done
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "sessionStatus", blocks[1].event)
			assert.Equal(t, "done", blocks[len(blocks)-1].event)

			var ss SessionStatusEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[1].data), &ss))
			assert.Equal(t, Active, ss.Status)
		})

		t.Run("unknownSession_404", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessID := fake.UUID().V4()
			userID := fake.Internet().User()
			notFound := errors.New("session not found")

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ReadSession(mock.Anything, mock.Anything).Return(nil, notFound)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, sessionPath(sessID), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusNotFound, *pd.Status)
		})

		t.Run("callerIdentityAbsent_401", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessID := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, sessionPath(sessID), nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusUnauthorized, *pd.Status)
		})
	})

	t.Run("ContinueAgentRun", func(t *testing.T) {
		continuePath := func(sessionID string) string {
			return "/sessions/" + sessionID + "/agent-runs"
		}

		makeReq := func(t *testing.T, ctx context.Context, msg, profileName, path string) *http.Request {
			t.Helper()
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			if profileName != "" {
				body = fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			}
			return httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == sessPath &&
					p.Message != nil &&
					p.Model == "" &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, continuePath(sessPath))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

			blocks := parseSSEBlocks(rec.Body.String())
			require.GreaterOrEqual(t, len(blocks), 2)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "done", blocks[len(blocks)-1].event)

			var sb SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sb))
			assert.Equal(t, sessPath, sb.SessionId)
		})

		t.Run("malformedJSON_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessPath := fake.UUID().V4()
			userID := fake.Internet().User()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(`{`))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusBadRequest, *pd.Status)
		})

		t.Run("callerIdentityAbsent_401", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			req := makeReq(t, t.Context(), msg, "", continuePath(sessPath))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Status)
			assert.Equal(t, http.StatusUnauthorized, *pd.Status)
		})

		t.Run("emptySessionIdPath_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, continuePath("%20%20"))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "sessionId")
		})

		t.Run("runnerError_500", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			runErr := errors.New(fake.Lorem().Sentence(4))
			profileName := "profile-" + fake.Lorem().Word()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, profileName, continuePath(sessPath))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent run failed")
		})

		t.Run("missing_profile_and_model_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "model is required when profileName is not provided")
		})

		t.Run("missing_profileName_uses_request_model", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			modelName := "myprovider/" + fake.Lorem().Word()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == sessPath &&
					p.Message != nil &&
					p.Model == modelName
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"model":%q,"message":{"parts":[{"text":%q}]}}`, modelName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("profileName_dispatches_regular_profile_default_model", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == sessPath &&
					p.Model == "" &&
					p.ProfileName == profileName
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(
				`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`,
				profileName,
				msg,
			)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("blank_profileName_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":"   ","message":{"parts":[{"text":%q}]}}`, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "profileName must not be blank")
		})

		t.Run("unknown_profileName_returns_404", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, &rt.AgentExecError{
				Kind: rt.AgentExecErrorKindNotFound,
				Op:   "load-profile",
				Err:  ap.ErrAgentProfileNotFound,
			})
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent profile not found")
		})

		t.Run("stream_event_error_is_written_to_sse", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()
			streamErr := fake.Lorem().Sentence(4)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.ProfileName == profileName && p.UserID == userID && p.SessionID == sessPath
			})).Return(rt.NewRunResult(func(yield func(*rt.SessionEvent, error) bool) {
				_ = yield(&rt.SessionEvent{
					ErrorCode:    "stream-protocol",
					ErrorMessage: streamErr,
				}, nil)
			}, sessPath), nil)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "error", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)
			assert.Contains(t, blocks[1].data, streamErr)
		})

		t.Run("profile_dispatch_validation_error_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, &rt.AgentExecError{
				Kind: rt.AgentExecErrorKindUnsupported,
				Op:   "dispatch-profile",
				Err:  errors.New("unsupported profile"),
			})
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, continuePath(sessPath), strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Equal(t, "invalid profile selection", *pd.Detail)
			assert.NotContains(t, *pd.Detail, "unsupported profile")
		})
	})

	t.Run("ReadSession", func(t *testing.T) {
		t.Run("blank_session_id_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())

			req := httptest.NewRequestWithContext(
				callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID}),
				http.MethodGet,
				"/sessions/%20%20",
				nil,
			)
			rec := httptest.NewRecorder()

			srv.ReadSession(rec, req, "   ")

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionId is required")
		})

		t.Run("stream_error_is_written_to_sse", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			sessionID := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().ReadSession(mock.Anything, mock.Anything).Return(
				rt.NewReadSessionResult(sessionID, false, func(yield func(*rt.SessionEvent, error) bool) {
					_ = yield(nil, errors.New("history failed"))
				}),
				nil,
			)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			req := httptest.NewRequestWithContext(
				callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID}),
				http.MethodGet,
				"/sessions/"+sessionID,
				nil,
			)
			rec := httptest.NewRecorder()

			srv.ReadSession(rec, req, sessionID)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "event: error")
		})
	})
}
