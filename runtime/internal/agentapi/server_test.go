//go:build !release

package agentapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/callerid"
	"github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/profileexec"
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

		var profileRunDispatcher agent.ProfileRunDispatcher
		if runner != nil {
			dispatcher, err := agent.NewProfileRunDispatcher(profilesSvc, runner)
			require.NoError(t, err)
			profileRunDispatcher = dispatcher
		}

		return NewAgentAPIServer(ServerParams{
			Runner:                 runner,
			Logger:                 log,
			IDGen:                  gen,
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
			ProfileRunDispatcher:   profileRunDispatcher,
			AgentProfilesService:   profilesSvc,
		})
	}
	newTestAgentAPIServer := func(t *testing.T, runner agent.AgentRunner, gen IDGen) *AgentAPIServer {
		t.Helper()
		return newTestAgentAPIServerWithProfiles(t, runner, gen, nil)
	}
	newTestAgentAPIServerWithDispatcher := func(
		t *testing.T,
		dispatcher agent.ProfileRunDispatcher,
		gen IDGen,
	) *AgentAPIServer {
		t.Helper()

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		return NewAgentAPIServer(ServerParams{
			Logger:                 log,
			IDGen:                  gen,
			RequestMapper:          NewAgentAPIRequestMapper(),
			SSEWriter:              NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper()),
			ProvidersConfigService: llmproviders.NewMockProvidersConfigService(t),
			ProfileRunDispatcher:   dispatcher,
			AgentProfilesService:   &mockAgentProfilesService{},
		})
	}
	newACPProfileServer := func(
		t *testing.T,
		gen IDGen,
		profilesSvc *mockAgentProfilesService,
	) http.Handler {
		t.Helper()

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		providersSvc := llmproviders.NewMockProvidersConfigService(t)
		providersSvc.EXPECT().List(mock.Anything).Return([]llmproviders.ProviderConfig{}, nil).Maybe()
		runner, err := agent.NewRunner(
			agent.RunnerArgs{ProvidersConfigService: providersSvc},
			agent.WithLogger(log),
		)
		require.NoError(t, err)

		srv := newTestAgentAPIServerWithProfiles(t, runner, gen, profilesSvc)
		return HandlerFromMux(srv, http.NewServeMux())
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

		makeACPProfile := func(name string, args []string) *ap.AgentProfile {
			return &ap.AgentProfile{
				Name: name,
				ExecutionSettings: ap.ExecutionSettings{
					Mode: ap.ExecutionModeACPStdio,
					AgentCommand: ap.ACPStdioAgentCommand{
						Command: os.Args[0],
						Args:    args,
					},
				},
			}
		}

		makeRegularProfile := func(name, defaultModel string) *ap.AgentProfile {
			return &ap.AgentProfile{
				Name: name,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: defaultModel,
				},
			}
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			profileModel := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == expSID &&
					p.Message != nil &&
					p.Model == profileModel
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)

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
			profileModel := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()

			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: chunk}}}
			ev.Partial = true

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Model == profileModel
			})).Return(fakeRunResult(expSID, []*session.Event{ev}), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()

			gen := NewMockIDGen()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()
			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()
			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(&ap.AgentProfile{
				Name: profileName,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: profileModel,
				},
			}, nil)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Model == profileModel
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)
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
			profileModel := "otherprovider/" + fake.Lorem().Word()
			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(&ap.AgentProfile{
				Name: profileName,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: profileModel,
				},
			}, nil)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == expSID && p.Model == overrideModel
			})).Return(fakeRunResult(expSID, nil), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, gen, profilesSvc)
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

		t.Run("profileName_without_dispatcher_returns_500", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			srv := newTestAgentAPIServerWithDispatcher(t, nil, NewMockIDGen())
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var pd ProblemDetails
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&pd))
			require.NotNil(t, pd.Detail)
			assert.Contains(t, *pd.Detail, "agent run failed")
		})

		t.Run("unknown_profileName_returns_404", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(nil, ap.ErrAgentProfileNotFound).Once()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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

		t.Run("acp_profile_streams_standard_sse", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			agentText := fake.Lorem().Sentence(4)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			srv := newTestAgentAPIServerWithDispatcher(t, stubProfileRunDispatcher{
				run: func(_ context.Context, req agent.ProfileRunRequest) (*agent.RunResult, error) {
					assert.Equal(t, profileName, req.ProfileName)
					assert.Equal(t, userID, req.UserID)
					assert.Equal(t, expSID, req.SessionID)
					return rt.NewRunResult(func(yield func(*rt.SessionEvent, error) bool) {
						_ = yield(&rt.SessionEvent{
							TurnComplete: true,
							Content: &rt.SessionEventContent{
								Role:  "model",
								Parts: []rt.SessionEventPart{{Text: agentText}},
							},
						}, nil)
					}, expSID), nil
				},
			}, gen)
			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "agent", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)
		})

		t.Run("acp_profile_ignores_request_model_override", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			overrideModel := "myprovider/" + fake.Lorem().Word()
			agentText := fake.Lorem().Sentence(4)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()

			srv := newTestAgentAPIServerWithDispatcher(t, stubProfileRunDispatcher{
				run: func(_ context.Context, req agent.ProfileRunRequest) (*agent.RunResult, error) {
					assert.Equal(t, profileName, req.ProfileName)
					assert.Equal(t, overrideModel, req.Model)
					assert.Equal(t, userID, req.UserID)
					assert.Equal(t, expSID, req.SessionID)
					return rt.NewRunResult(func(yield func(*rt.SessionEvent, error) bool) {
						_ = yield(&rt.SessionEvent{
							TurnComplete: true,
							Content: &rt.SessionEventContent{
								Role:  "model",
								Parts: []rt.SessionEventPart{{Text: agentText}},
							},
						}, nil)
					}, expSID), nil
				},
			}, gen)
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
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "agent", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)
		})

		t.Run("acp_profile_launch_success_replays_session_history", func(t *testing.T) {
			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			progressText := fake.Lorem().Sentence(2)
			finalText := fake.Lorem().Sentence(3)

			t.Setenv("SONALMOD_AGENTAPI_ACP_HELPER_MODE", "success")
			t.Setenv("SONALMOD_AGENTAPI_ACP_PROGRESS_TEXT", progressText)
			t.Setenv("SONALMOD_AGENTAPI_ACP_FINAL_TEXT", finalText)

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On(
				"Get",
				mock.Anything,
				profileName,
			).Return(
				makeACPProfile(
					profileName,
					[]string{"-test.run=TestAgentAPIServerACPHelperProcess", "--"},
				),
				nil,
			).Once()

			h := newACPProfileServer(t, gen, profilesSvc)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			runBlocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, runBlocks, 4)
			assert.Equal(t, "sessionBound", runBlocks[0].event)
			assert.Equal(t, "agent", runBlocks[1].event)
			assert.Equal(t, "agent", runBlocks[2].event)
			assert.Equal(t, "done", runBlocks[3].event)
			assert.Contains(t, runBlocks[1].data, progressText)
			assert.Contains(t, runBlocks[2].data, finalText)

			var sessionBound SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(runBlocks[0].data), &sessionBound))
			assert.Equal(t, expSID, sessionBound.SessionId)

			readReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/sessions/"+expSID, nil)
			readRec := httptest.NewRecorder()
			h.ServeHTTP(readRec, readReq)

			require.Equal(t, http.StatusOK, readRec.Code)
			readBlocks := parseSSEBlocks(readRec.Body.String())
			require.Len(t, readBlocks, 5)
			assert.Equal(t, "sessionBound", readBlocks[0].event)
			assert.Equal(t, "sessionStatus", readBlocks[1].event)
			assert.Equal(t, "agent", readBlocks[2].event)
			assert.Equal(t, "agent", readBlocks[3].event)
			assert.Equal(t, "done", readBlocks[4].event)
			assert.Contains(t, readBlocks[2].data, msg)
			assert.Contains(t, readBlocks[3].data, finalText)

			var status SessionStatusEvent
			require.NoError(t, json.Unmarshal([]byte(readBlocks[1].data), &status))
			assert.Equal(t, Idle, status.Status)
		})

		t.Run("acp_profile_launch_failure_streams_standard_error", func(t *testing.T) {
			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()

			gen := NewMockIDGen()
			expSID := MockIDGenNextGenerated(gen).String()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(&ap.AgentProfile{
				Name: profileName,
				ExecutionSettings: ap.ExecutionSettings{
					Mode: ap.ExecutionModeACPStdio,
					AgentCommand: ap.ACPStdioAgentCommand{
						Command: "/no/such/opencode-binary",
					},
				},
			}, nil).Once()

			h := newACPProfileServer(t, gen, profilesSvc)

			ctx := callerid.ContextWith(t.Context(), &fakeCallerIdentity{userID: userID})
			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/agent-runs", strings.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "error", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)
			assert.Contains(t, blocks[1].data, "acp-stdio-subprocess")

			var sessionBound SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sessionBound))
			assert.Equal(t, expSID, sessionBound.SessionId)
		})

		t.Run("integration_realAgentRunner", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			ctx := t.Context()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			profileName := "profile-" + fake.Lorem().Word()
			profileModel := "myprovider/" + fake.Lorem().Word()

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
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()
			srv := newTestAgentAPIServerWithProfiles(t, bgRunner, gen, profilesSvc)

			mux := http.NewServeMux()
			h := HandlerFromMux(srv, mux)

			body := fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
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

		makeReq := func(t *testing.T, ctx context.Context, msg, profileName, path string) *http.Request {
			t.Helper()
			body := fmt.Sprintf(`{"message":{"parts":[{"text":%q}]}}`, msg)
			if profileName != "" {
				body = fmt.Sprintf(`{"profileName":%q,"message":{"parts":[{"text":%q}]}}`, profileName, msg)
			}
			return httptest.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
		}

		makeRegularProfile := func(name, defaultModel string) *ap.AgentProfile {
			return &ap.AgentProfile{
				Name: name,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: defaultModel,
				},
			}
		}

		t.Run("success_SSE_sessionBound_and_done", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()
			profileModel := "myprovider/" + fake.Lorem().Word()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID &&
					p.SessionID == sessPath &&
					p.Message != nil &&
					p.Model == profileModel
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(
				makeRegularProfile(profileName, profileModel),
				nil,
			).Once()

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.Anything).Return(nil, runErr)

			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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
			profileModel := "myprovider/" + fake.Lorem().Word()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(&ap.AgentProfile{
				Name: profileName,
				ExecutionSettings: ap.ExecutionSettings{
					DefaultModel: profileModel,
				},
			}, nil)

			m := agent.NewMockAgentRunner(t)
			m.EXPECT().Run(mock.Anything, mock.MatchedBy(func(p rt.RunParams) bool {
				return p.UserID == userID && p.SessionID == sessPath && p.Model == profileModel
			})).Return(fakeRunResult(sessPath, nil), nil)

			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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

			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On("Get", mock.Anything, profileName).Return(nil, ap.ErrAgentProfileNotFound).Once()

			m := agent.NewMockAgentRunner(t)
			srv := newTestAgentAPIServerWithProfiles(t, m, NewMockIDGen(), profilesSvc)
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

		t.Run("acp_profile_streams_standard_error", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()
			streamErr := fake.Lorem().Sentence(4)

			srv := newTestAgentAPIServerWithDispatcher(t, stubProfileRunDispatcher{
				run: func(_ context.Context, req agent.ProfileRunRequest) (*agent.RunResult, error) {
					assert.Equal(t, profileName, req.ProfileName)
					assert.Equal(t, userID, req.UserID)
					assert.Equal(t, sessPath, req.SessionID)
					return rt.NewRunResult(func(yield func(*rt.SessionEvent, error) bool) {
						_ = yield(&rt.SessionEvent{
							ErrorCode:    "acp-stdio-protocol",
							ErrorMessage: streamErr,
						}, nil)
					}, sessPath), nil
				},
			}, NewMockIDGen())
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

		t.Run("acp_profile_protocol_failure_streams_standard_error", func(t *testing.T) {
			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()

			t.Setenv("SONALMOD_AGENTAPI_ACP_HELPER_MODE", "bad-initialize")
			gen := NewMockIDGen()
			profilesSvc := &mockAgentProfilesService{}
			profilesSvc.On(
				"Get",
				mock.Anything,
				profileName,
			).Return(
				&ap.AgentProfile{
					Name: profileName,
					ExecutionSettings: ap.ExecutionSettings{
						Mode: ap.ExecutionModeACPStdio,
						AgentCommand: ap.ACPStdioAgentCommand{
							Command: os.Args[0],
							Args:    []string{"-test.run=TestAgentAPIServerACPHelperProcess", "--"},
						},
					},
				},
				nil,
			).Once()

			h := newACPProfileServer(t, gen, profilesSvc)

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
			assert.Contains(t, blocks[1].data, "acp-stdio-protocol")

			var sessionBound SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sessionBound))
			assert.Equal(t, sessPath, sessionBound.SessionId)
		})

		t.Run("profile_dispatch_validation_error_returns_400", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			userID := fake.Internet().User()
			msg := fake.Lorem().Sentence(3)
			sessPath := fake.UUID().V4()
			profileName := "profile-" + fake.Lorem().Word()

			srv := newTestAgentAPIServerWithDispatcher(t, stubProfileRunDispatcher{
				run: func(context.Context, agent.ProfileRunRequest) (*agent.RunResult, error) {
					return nil, &profileexec.Error{
						Kind: profileexec.ErrorKindUnsupported,
						Op:   "dispatch-profile",
						Err:  errors.New("unsupported profile"),
					}
				},
			}, NewMockIDGen())
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
			assert.Contains(t, *pd.Detail, "unsupported profile")
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

// smokeIntegrationFakeLLM implements model.LLM for wiring *AgentRunner through HTTP without a live LLM (same pattern as internal/agentrun_test.go fakeModel).
type smokeIntegrationFakeLLM struct{ name string }

type stubProfileRunDispatcher struct {
	run func(ctx context.Context, request agent.ProfileRunRequest) (*agent.RunResult, error)
}

func (d stubProfileRunDispatcher) Run(
	ctx context.Context,
	request agent.ProfileRunRequest,
) (*agent.RunResult, error) {
	return d.run(ctx, request)
}

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

func TestAgentAPIServerACPHelperProcess(_ *testing.T) {
	mode := os.Getenv("SONALMOD_AGENTAPI_ACP_HELPER_MODE")
	if mode == "" {
		return
	}

	progressText := os.Getenv("SONALMOD_AGENTAPI_ACP_PROGRESS_TEXT")
	if progressText == "" {
		progressText = "thinking"
	}
	finalText := os.Getenv("SONALMOD_AGENTAPI_ACP_FINAL_TEXT")
	if finalText == "" {
		finalText = "done"
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req map[string]any
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			fmt.Fprintf(
				os.Stdout,
				"{\"jsonrpc\":\"2.0\",\"error\":{\"code\":-32700,\"message\":\"%s\"}}\n",
				err.Error(),
			)
			continue
		}

		id := req["id"]
		method, _ := req["method"].(string)
		switch mode {
		case "success":
			switch method {
			case "initialize":
				writeACPHelperResult(id, map[string]any{"capabilities": map[string]any{}})
			case "session/new":
				writeACPHelperResult(id, map[string]any{"sessionId": "session-1"})
			case "session/prompt":
				writeACPHelperNotification("session/update", map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"type":    "progress",
						"message": progressText,
					},
				})
				writeACPHelperNotification("session/update", map[string]any{
					"sessionId": "session-1",
					"update": map[string]any{
						"type":    "final",
						"message": finalText,
					},
				})
				writeACPHelperResult(id, map[string]any{"ok": true})
			default:
				writeACPHelperError(id, -32601, "Method not found")
			}
		case "bad-initialize":
			if method == "initialize" {
				writeACPHelperResult(id, "bad-result")
				continue
			}
			writeACPHelperError(id, -32601, "Method not found")
		default:
			writeACPHelperError(id, -32603, "Unknown helper mode")
		}
	}

	os.Exit(0)
}

func writeACPHelperResult(id any, result any) {
	encoded, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	fmt.Fprintf(os.Stdout, "%s\n", encoded)
}

func writeACPHelperNotification(method string, params any) {
	encoded, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	fmt.Fprintf(os.Stdout, "%s\n", encoded)
}

func writeACPHelperError(id any, code int, message string) {
	encoded, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	fmt.Fprintf(os.Stdout, "%s\n", encoded)
}
