//go:build !release

package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	"github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// fakeCallerIdentity is a test implementation of callerid.Identity.
type fakeCallerIdentity struct{ userID string }

func (f *fakeCallerIdentity) UserID() string { return f.userID }

func TestAgentAPIServer(t *testing.T) {
	newTestAgentAPIServer := func(t *testing.T, runner agent.AgentRunner, gen IDGen) *AgentAPIServer {
		t.Helper()
		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		return NewAgentAPIServer(ServerParams{
			Runner:                 runner,
			Logger:                 log,
			IDGen:                  gen,
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
			AgentProfilesService:   &mockAgentProfilesService{},
		})
	}

	t.Run("StartAgentRun", func(t *testing.T) {
		makeReq := func(t *testing.T, ctx context.Context, msg, path string) *http.Request {
			t.Helper()
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			return httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Message != nil
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)

			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, "/agent-runs")
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

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: chunk}}}
			ev.Partial = true

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID
			})).Return(fakeRunResult(expSID, []*session.Event{ev}), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, "/agent-runs")
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

			req := makeReq(t, t.Context(), msg, "/agent-runs")
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

			gen := NewMockIDGen()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, "/agent-runs")
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

			gen := NewMockIDGen()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, "/agent-runs")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// StreamAgentRun(nil) returns before writing SSE; handler only logs the error.
			assert.Empty(t, rec.Header().Get("Content-Type"))
		})

		t.Run("model_field_passed_to_RunParams", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			modelName := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Model == modelName
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

		t.Run("no_model_field_RunParams_Model_empty", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Model == ""
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServer(t, m, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "sessionBound")
		})

		t.Run("integration_realAgentRunner", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			ctx := t.Context()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			f := rt.NewAgentRunnerFactory(rt.AgentRunnerFactoryDeps{
				LLMAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return &smokeIntegrationFakeLLM{}, nil
				},
				LLMAgentFactory:       llmagent.New,
				LLMAgentRunnerFactory: rt.RunExecutorFactoryFromRunner,
				SessionStorage:        sessions.NewMemorySessionsStorage(),
				RootLogger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
			})

			ar, err := f.NewAgentRunner(ctx, rt.NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: fake.Lorem().Word(),
				SystemPromptFragments: []rt.SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: fake.Lorem().Sentence(3)},
				},
				ToolsRegistry: rt.StaticTools(nil),
				ModelName:     "",
			})
			require.NoError(t, err)
			require.NotNil(t, ar)

			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			bgRunner := rt.NewBackgroundRunner(rt.BackgroundRunnerDeps{
				Runner: ar,
				Logger: log,
			})
			srv := NewAgentAPIServer(ServerParams{
				Runner:                 bgRunner,
				Logger:                 log,
				IDGen:                  gen,
				RequestMapper:          NewAgentAPIRequestMapper(),
				SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
				ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
				AgentProfilesService:   &mockAgentProfilesService{},
			})

			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			reqCtx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := httptest.NewRequestWithContext(reqCtx, http.MethodPost, "/agent-runs", strings.NewReader(body))
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

			bodyStr := rec.Body.String()
			assert.Contains(t, bodyStr, `"event":"agent"`)
			assert.Contains(t, bodyStr, "smoke-ok")
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

		makeReq := func(t *testing.T, ctx context.Context, msg, path string) *http.Request {
			t.Helper()
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			return httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == sessPath && p.Message != nil
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, continuePath(sessPath))
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

			req := makeReq(t, t.Context(), msg, continuePath(sessPath))
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

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, continuePath("%20%20"))
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

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServer(t, m, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			req := makeReq(t, ctx, msg, continuePath(sessPath))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent run failed")
		})

		t.Run("model_field_passed_to_RunParams", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			modelName := "myprovider/" + fake.Lorem().Word()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == sessPath && p.Model == modelName
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
	})
}

// smokeIntegrationFakeLLM implements model.LLM for wiring *AgentRunner through HTTP without a live LLM (same pattern as internal/agentrun_test.go fakeModel).
type smokeIntegrationFakeLLM struct{ name string }

func (m *smokeIntegrationFakeLLM) Name() string {
	if m.name != "" {
		return m.name
	}
	return "fake"
}

func (m *smokeIntegrationFakeLLM) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "smoke-ok"}},
			},
		}, nil)
	}
}
