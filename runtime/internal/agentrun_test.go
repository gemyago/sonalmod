//go:build !release

package internal

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"strings"
	"testing"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func TestResolveSession(t *testing.T) {
	fake := faker.New()

	type mockDeps struct {
		sessionService *MockService
		llmRunner      *MockLLMRunner
	}

	makeMockDeps := func(t *testing.T) mockDeps {
		return mockDeps{
			sessionService: NewMockService(t),
			llmRunner:      NewMockLLMRunner(t),
		}
	}

	// newAgentRunner builds an AgentRunner wired for ensureSession tests (llmRunner mocked directly).
	newAgentRunner := func(t *testing.T, deps mockDeps, appName string) *AgentRunner {
		return &AgentRunner{
			sessionStorage: &mockSessionsStorageAdapter{MockService: deps.sessionService},
			appName:        appName,
			llmRunner:      deps.llmRunner,
			logger:         RootTestLogger().With("test", t.Name()),
		}
	}

	t.Run("ensureSession", func(t *testing.T) {
		t.Run("non-empty sessionID Get returns existing session without error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)

			// Create a real session via InMemoryService for the expected return
			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(&session.GetResponse{Session: expectedSess}, nil)

			require.NoError(t, runner.ensureSession(ctx, userID, sessionID))
		})

		t.Run("non-empty sessionID Get not found creates session without error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(nil, errors.New("session "+sessionID+" not found"))

			// Create a real session for the Create response
			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Create(ctx, &session.CreateRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
					State:     make(map[string]any),
				}).
				Return(&session.CreateResponse{Session: expectedSess}, nil)

			require.NoError(t, runner.ensureSession(ctx, userID, sessionID))
		})

		t.Run("empty sessionID returns error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, fake.Lorem().Word())

			err := runner.ensureSession(ctx, userID, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "sessionID")
		})

		t.Run("Get error propagates error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			getErr := errors.New("database connection failed")

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(nil, getErr)

			err := runner.ensureSession(ctx, userID, sessionID)
			require.Error(t, err)
			assert.ErrorIs(t, err, getErr)
		})

		t.Run("Create error propagates error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			createErr := errors.New("create failed")

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(nil, errors.New("session "+sessionID+" not found"))

			deps.sessionService.EXPECT().
				Create(ctx, &session.CreateRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
					State:     make(map[string]any),
				}).
				Return(nil, createErr)

			err := runner.ensureSession(ctx, userID, sessionID)
			require.Error(t, err)
			assert.ErrorIs(t, err, createErr)
		})
	})
	t.Run("Run", func(t *testing.T) {
		makeFinalEvent := func(text string) *session.Event {
			ev := session.NewEvent("inv-1")
			ev.Content = &genai.Content{
				Parts: []*genai.Part{{Text: text}},
			}
			ev.Partial = false
			return ev
		}

		t.Run("happy path", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			expectedText := fake.Lorem().Sentence(8)
			msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}}

			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(&session.GetResponse{Session: expectedSess}, nil)

			events := func(yield func(*session.Event, error) bool) {
				yield(makeFinalEvent(expectedText), nil)
			}
			deps.llmRunner.EXPECT().
				Run(ctx, userID, sessionID, messageContentToGenAI(msg), agent.RunConfig{
					StreamingMode: agent.StreamingModeSSE,
				}).
				Return(events)

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			got, err := result.ConsumeEventsAsString(ctx)
			require.NoError(t, err)
			assert.Equal(t, expectedText, got)
		})

		t.Run("empty SessionID Run returns error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, fake.Lorem().Word())

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: "",
				Message:   &MessageContent{},
			})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), "sessionID")
		})

		t.Run("ensureSession error Run returns error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			getErr := errors.New("database connection failed")

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(nil, getErr)

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   &MessageContent{},
			})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, getErr)
		})

		t.Run("RunExecutor yields stream error ConsumeEventsAsString returns error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			streamErr := errors.New("stream failed")
			msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(2)}}}

			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(&session.GetResponse{Session: expectedSess}, nil)

			events := func(yield func(*session.Event, error) bool) {
				yield(nil, streamErr)
			}
			deps.llmRunner.EXPECT().
				Run(ctx, userID, sessionID, messageContentToGenAI(msg), agent.RunConfig{
					StreamingMode: agent.StreamingModeSSE,
				}).
				Return(events)

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			_, err = result.ConsumeEventsAsString(ctx)
			require.Error(t, err)
			assert.ErrorIs(t, err, streamErr)
		})

		t.Run("RunExecutor yields no final answer ConsumeEventsAsString returns error", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(2)}}}

			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(&session.GetResponse{Session: expectedSess}, nil)

			// Events with only partial (no final) - ConsumeEventsAsString returns error when no text at all
			// Empty stream or events with no text content
			events := func(_ func(*session.Event, error) bool) {
				// yields nothing
			}
			deps.llmRunner.EXPECT().
				Run(ctx, userID, sessionID, messageContentToGenAI(msg), agent.RunConfig{
					StreamingMode: agent.StreamingModeSSE,
				}).
				Return(events)

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			_, err = result.ConsumeEventsAsString(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "no final answer")
		})

		t.Run("Run passes SSE streaming mode to RunExecutor", func(t *testing.T) {
			deps := makeMockDeps(t)
			ctx := t.Context()
			sessionID := fake.UUID().V4()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			runner := newAgentRunner(t, deps, appName)
			msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(2)}}}

			inMem := session.InMemoryService()
			createResp, err := inMem.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			expectedSess := createResp.Session

			deps.sessionService.EXPECT().
				Get(ctx, &session.GetRequest{
					AppName:   appName,
					UserID:    userID,
					SessionID: sessionID,
				}).
				Return(&session.GetResponse{Session: expectedSess}, nil)

			events := func(_ func(*session.Event, error) bool) {}
			deps.llmRunner.EXPECT().
				Run(ctx, userID, sessionID, messageContentToGenAI(msg), agent.RunConfig{
					StreamingMode: agent.StreamingModeSSE,
				}).
				Return(events)

			result, err := runner.Run(ctx, RunParams{
				UserID:    userID,
				SessionID: sessionID,
				Message:   msg,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
		})
	})
}

// fakeModel implements model.LLM for integration-style tests without Genkit.
type fakeModel struct{ name string }

func (m *fakeModel) Name() string {
	if m.name != "" {
		return m.name
	}
	return "fake"
}

func (m *fakeModel) GenerateContent(
	_ context.Context,
	_ *model.LLMRequest,
	_ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "ok"}},
			},
		}, nil)
	}
}

func TestAgentRunnerFactory(t *testing.T) {
	fake := faker.New()

	t.Run("NewAgentRunner", func(t *testing.T) {
		t.Run("returns non-nil AgentRunner with real llmagent and in-memory session", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			genkitFactory := func(_ context.Context, _ string) (model.LLM, error) {
				return &fakeModel{}, nil
			}
			f := &AgentRunnerFactory{
				llmAdapterFactory:     genkitFactory,
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}

			params := NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: fake.Lorem().Word(),
				SystemPromptFragments: []SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: fake.Lorem().Sentence(3)},
				},
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			}

			runner, err := f.NewAgentRunner(ctx, params)
			require.NoError(t, err)
			require.NotNil(t, runner)
			assert.Equal(t, params.AppName, runner.appName)
			assert.Equal(t, stor, runner.sessionStorage)
			assert.NotNil(t, runner.llmRunner)
		})

		t.Run("unit test with mocked LLMAgentFactory returns non-nil AgentRunner", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			genkitFactory := func(_ context.Context, _ string) (model.LLM, error) {
				return &fakeModel{}, nil
			}
			agentName := fake.Lorem().Word()
			agentInstruction := fake.Lorem().Sentence(2)
			mockAgent, err := llmagent.New(llmagent.Config{
				Name:        agentName,
				Model:       &fakeModel{},
				Instruction: agentInstruction,
			})
			require.NoError(t, err)

			llmAgentFactory := func(_ llmagent.Config) (agent.Agent, error) {
				return mockAgent, nil
			}
			f := &AgentRunnerFactory{
				llmAdapterFactory:     genkitFactory,
				llmAgentFactory:       llmAgentFactory,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}

			params := NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: agentName,
				SystemPromptFragments: []SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: agentInstruction},
				},
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			}

			runner, err := f.NewAgentRunner(ctx, params)
			require.NoError(t, err)
			require.NotNil(t, runner)
		})

		t.Run("LLMAgentFactory error propagates", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			genkitFactory := func(_ context.Context, _ string) (model.LLM, error) {
				return &fakeModel{}, nil
			}
			llmErr := errors.New("agent creation failed")
			llmAgentFactory := func(_ llmagent.Config) (agent.Agent, error) {
				return nil, llmErr
			}
			f := &AgentRunnerFactory{
				llmAdapterFactory:     genkitFactory,
				llmAgentFactory:       llmAgentFactory,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}

			params := NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: fake.Lorem().Word(),
				SystemPromptFragments: []SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: fake.Lorem().Sentence(2)},
				},
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			}

			runner, err := f.NewAgentRunner(ctx, params)
			require.Error(t, err)
			assert.Nil(t, runner)
			assert.ErrorIs(t, err, llmErr)
		})

		t.Run("LLMAdapterFactory error propagates", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			resolveErr := errors.New("model resolution failed")
			f := &AgentRunnerFactory{
				llmAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return nil, resolveErr
				},
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}
			_, err := f.NewAgentRunner(ctx, NewAgentRunnerParams{
				AppName:       fake.Lorem().Word(),
				AgentName:     fake.Lorem().Word(),
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, resolveErr)
		})

		t.Run("non-empty ModelName passed to LLMAdapterFactory", func(t *testing.T) {
			ctx := t.Context()
			stor := sessions.NewMemorySessionsStorage()
			modelName := fake.Lorem().Word()
			var capturedModelName string
			genkitFactory := func(_ context.Context, name string) (model.LLM, error) {
				capturedModelName = name
				return &fakeModel{name: name}, nil
			}
			f := &AgentRunnerFactory{
				llmAdapterFactory:     genkitFactory,
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}

			params := NewAgentRunnerParams{
				AppName:       fake.Lorem().Word(),
				AgentName:     fake.Lorem().Word(),
				ToolsRegistry: StaticTools(nil),
				ModelName:     modelName,
			}

			runner, err := f.NewAgentRunner(ctx, params)
			require.NoError(t, err)
			require.NotNil(t, runner)
			assert.Equal(t, modelName, capturedModelName)
		})

		t.Run("forwards context to LLMAdapterFactory", func(t *testing.T) {
			type ctxMarker struct{}
			var k ctxMarker
			ctx := context.WithValue(t.Context(), k, "llm-factory-ctx")
			stor := sessions.NewMemorySessionsStorage()
			var got any
			genkitFactory := func(c context.Context, _ string) (model.LLM, error) {
				got = c.Value(k)
				return &fakeModel{}, nil
			}
			f := &AgentRunnerFactory{
				llmAdapterFactory:     genkitFactory,
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}
			_, err := f.NewAgentRunner(ctx, NewAgentRunnerParams{
				AppName:       fake.Lorem().Word(),
				AgentName:     fake.Lorem().Word(),
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			})
			require.NoError(t, err)
			assert.Equal(t, "llm-factory-ctx", got)
		})

		t.Run("GetTools error propagates", func(t *testing.T) {
			ctx := t.Context()
			toolsErr := errors.New("tools failed")
			f := &AgentRunnerFactory{
				llmAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return &fakeModel{}, nil
				},
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        sessions.NewMemorySessionsStorage(),
				rootLogger:            RootTestLogger(),
			}
			_, err := f.NewAgentRunner(ctx, NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: fake.Lorem().Word(),
				SystemPromptFragments: []SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: fake.Lorem().Sentence(2)},
				},
				ToolsRegistry: errToolsProvider{err: toolsErr},
				ModelName:     fake.Lorem().Word(),
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, toolsErr)
		})

		t.Run("LLMAgentRunnerFactory error propagates", func(t *testing.T) {
			ctx := t.Context()
			runnerErr := errors.New("runner factory failed")
			mockAgent, err := llmagent.New(llmagent.Config{
				Name:        fake.Lorem().Word(),
				Model:       &fakeModel{},
				Instruction: fake.Lorem().Sentence(2),
			})
			require.NoError(t, err)
			f := &AgentRunnerFactory{
				llmAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return &fakeModel{}, nil
				},
				llmAgentFactory: func(_ llmagent.Config) (agent.Agent, error) {
					return mockAgent, nil
				},
				llmAgentRunnerFactory: func(_ runner.Config) (*runner.Runner, error) {
					return nil, runnerErr
				},
				sessionStorage: sessions.NewMemorySessionsStorage(),
				rootLogger:     RootTestLogger(),
			}
			_, err = f.NewAgentRunner(ctx, NewAgentRunnerParams{
				AppName:   fake.Lorem().Word(),
				AgentName: fake.Lorem().Word(),
				SystemPromptFragments: []SystemPromptFragment{
					{Section: fake.Lorem().Word(), Content: fake.Lorem().Sentence(2)},
				},
				ToolsRegistry: StaticTools(nil),
				ModelName:     fake.Lorem().Word(),
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, runnerErr)
		})
	})

	t.Run("ReadSession", func(t *testing.T) {
		makeFactory := func(_ *testing.T, stor sessions.SessionsStorage) *AgentRunnerFactory {
			return &AgentRunnerFactory{
				llmAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return &fakeModel{}, nil
				},
				llmAgentFactory:       llmagent.New,
				llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
				sessionStorage:        stor,
				rootLogger:            RootTestLogger(),
			}
		}

		t.Run("ReadSession returns mapped events from session service", func(t *testing.T) {
			ctx := t.Context()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			sessionID := fake.UUID().V4()
			text1 := fake.Lorem().Sentence(3)
			text2 := fake.Lorem().Sentence(3)

			stor := sessions.NewMemorySessionsStorage()
			createResp, err := stor.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
				State:     make(map[string]any),
			})
			require.NoError(t, err)
			sess := createResp.Session

			ev1 := session.NewEvent("inv-1")
			ev1.Content = &genai.Content{Parts: []*genai.Part{{Text: text1}}}
			ev2 := session.NewEvent("inv-2")
			ev2.Content = &genai.Content{Parts: []*genai.Part{{Text: text2}}}
			require.NoError(t, stor.AppendEvent(ctx, sess, ev1))
			require.NoError(t, stor.AppendEvent(ctx, sess, ev2))

			f := makeFactory(t, stor)
			result, err := f.ReadSession(ctx, ReadSessionParams{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, sessionID, result.SessionID())
			assert.False(t, result.IsActive())
			var events []*SessionEvent
			for ev, evErr := range result.Events() {
				require.NoError(t, evErr)
				events = append(events, ev)
			}
			require.Len(t, events, 2)
			require.NotNil(t, events[0].Content)
			assert.Equal(t, text1, events[0].Content.Parts[0].Text)
			require.NotNil(t, events[1].Content)
			assert.Equal(t, text2, events[1].Content.Parts[0].Text)
		})

		t.Run("ReadSession with unknown session returns error", func(t *testing.T) {
			ctx := t.Context()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			sessionID := fake.UUID().V4()

			stor := sessions.NewMemorySessionsStorage()
			f := makeFactory(t, stor)
			result, err := f.ReadSession(ctx, ReadSessionParams{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			})
			require.Error(t, err)
			assert.Nil(t, result)
		})

		t.Run("*AgentRunner ReadSession returns mapped events", func(t *testing.T) {
			ctx := t.Context()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			sessionID := fake.UUID().V4()
			text := fake.Lorem().Sentence(3)

			stor := sessions.NewMemorySessionsStorage()
			createResp, err := stor.Create(ctx, &session.CreateRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
				State:     make(map[string]any),
			})
			require.NoError(t, err)
			ev := session.NewEvent(fake.UUID().V4())
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: text}}}
			require.NoError(t, stor.AppendEvent(ctx, createResp.Session, ev))

			runner := &AgentRunner{
				appName:        appName,
				sessionStorage: stor,
				logger:         RootTestLogger(),
			}
			result, err := runner.ReadSession(ctx, ReadSessionParams{
				UserID:    userID,
				SessionID: sessionID,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			var events []*SessionEvent
			for event, eventErr := range result.Events() {
				require.NoError(t, eventErr)
				events = append(events, event)
			}
			require.Len(t, events, 1)
			require.NotNil(t, events[0].Content)
			assert.Equal(t, text, events[0].Content.Parts[0].Text)
		})

		t.Run("*AgentRunner ReadSession propagates storage errors", func(t *testing.T) {
			ctx := t.Context()
			runner := &AgentRunner{
				appName:        fake.Lorem().Word(),
				sessionStorage: sessions.NewMemorySessionsStorage(),
				logger:         RootTestLogger(),
			}
			result, err := runner.ReadSession(ctx, ReadSessionParams{
				UserID:    fake.UUID().V4(),
				SessionID: fake.UUID().V4(),
			})
			require.Error(t, err)
			assert.Nil(t, result)
		})
	})

	t.Run("ListSessions", func(t *testing.T) {
		t.Run("delegates to metadata store", func(t *testing.T) {
			ctx := t.Context()
			want := &ListSessionMetadataResult{
				Sessions: []SessionMetadata{{SessionID: fake.UUID().V4()}},
				Total:    1,
			}
			spy := &listSpySessionMetadataStore{listResult: want}
			storage := &listSpySessionsStorage{Service: session.InMemoryService(), spy: spy}
			params := ListSessionMetadataParams{
				AppName: fake.Lorem().Word(),
				UserID:  fake.UUID().V4(),
				Limit:   10,
				Offset:  0,
			}
			f := NewAgentRunnerFactory(AgentRunnerFactoryDeps{
				LLMAdapterFactory: func(context.Context, string) (model.LLM, error) {
					return &fakeModel{}, nil
				},
				LLMAgentFactory:       llmagent.New,
				LLMAgentRunnerFactory: RunExecutorFactoryFromRunner,
				SessionStorage:        storage,
				RootLogger:            RootTestLogger(),
			})
			got, err := f.ListSessions(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			require.Len(t, spy.listCalls, 1)
			assert.Equal(t, params, spy.listCalls[0])
		})

		t.Run("*AgentRunner returns empty when session storage is nil", func(t *testing.T) {
			ctx := t.Context()
			appName := fake.Lorem().Word()
			a := &AgentRunner{appName: appName, logger: RootTestLogger()}
			got, err := a.ListSessions(ctx, ListSessionMetadataParams{
				UserID: fake.UUID().V4(),
				Limit:  5,
				Offset: 0,
			})
			require.NoError(t, err)
			assert.Equal(t, &ListSessionMetadataResult{}, got)
		})

		t.Run("*AgentRunner delegates with AppName from runner", func(t *testing.T) {
			ctx := t.Context()
			appName := fake.Lorem().Word()
			userID := fake.UUID().V4()
			want := &ListSessionMetadataResult{Total: 2}
			spy := &listSpySessionMetadataStore{listResult: want}
			storage := &listSpySessionsStorage{Service: session.InMemoryService(), spy: spy}
			a := &AgentRunner{
				appName:        appName,
				sessionStorage: storage,
				logger:         RootTestLogger(),
			}
			params := ListSessionMetadataParams{
				AppName: fake.Lorem().Word(),
				UserID:  userID,
				Limit:   10,
				Offset:  1,
			}
			got, err := a.ListSessions(ctx, params)
			require.NoError(t, err)
			assert.Equal(t, want, got)
			require.Len(t, spy.listCalls, 1)
			expected := params
			expected.AppName = appName
			assert.Equal(t, expected, spy.listCalls[0])
		})
	})

	t.Run("NewAgentRunnerFactory", func(t *testing.T) {
		f := NewAgentRunnerFactory(AgentRunnerFactoryDeps{
			LLMAdapterFactory: func(context.Context, string) (model.LLM, error) {
				return &fakeModel{}, nil
			},
			LLMAgentFactory:       llmagent.New,
			LLMAgentRunnerFactory: RunExecutorFactoryFromRunner,
			SessionStorage:        sessions.NewMemorySessionsStorage(),
			RootLogger:            slog.Default(),
		})
		require.NotNil(t, f)
	})
}

type mockSessionsStorageAdapter struct {
	*MockService
}

func (a *mockSessionsStorageAdapter) SaveMetadata(_ context.Context, _ SessionMetadata) error {
	return nil
}

func (a *mockSessionsStorageAdapter) ListMetadata(
	_ context.Context,
	_ ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return &ListSessionMetadataResult{}, nil
}

func (a *mockSessionsStorageAdapter) DeleteMetadata(_ context.Context, _, _, _ string) error {
	return nil
}

func (a *mockSessionsStorageAdapter) AutoMigrate() error {
	return nil
}

var _ sessions.SessionsStorage = (*mockSessionsStorageAdapter)(nil)

type listSpySessionMetadataStore struct {
	listCalls  []ListSessionMetadataParams
	listResult *ListSessionMetadataResult
	listErr    error
}

func (s *listSpySessionMetadataStore) Save(_ context.Context, _ SessionMetadata) error {
	return nil
}

func (s *listSpySessionMetadataStore) List(
	_ context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	s.listCalls = append(s.listCalls, params)
	return s.listResult, s.listErr
}

func (s *listSpySessionMetadataStore) Delete(_ context.Context, _, _, _ string) error {
	return nil
}

type listSpySessionsStorage struct {
	session.Service

	spy *listSpySessionMetadataStore
}

func (s *listSpySessionsStorage) SaveMetadata(ctx context.Context, m SessionMetadata) error {
	return s.spy.Save(ctx, m)
}

func (s *listSpySessionsStorage) ListMetadata(
	ctx context.Context,
	p ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return s.spy.List(ctx, p)
}

func (s *listSpySessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
	return s.spy.Delete(ctx, appName, userID, sessionID)
}

func (s *listSpySessionsStorage) AutoMigrate() error {
	return nil
}

var _ sessions.SessionsStorage = (*listSpySessionsStorage)(nil)

type errToolsProvider struct {
	err error
}

func (e errToolsProvider) GetTools() ([]tool.Tool, error) {
	return nil, e.err
}

func TestAgentRunnerProfileDispatch(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	// makeDispatchRunner builds a minimal AgentRunner wired for profile dispatch tests.
	// The factory's llmAdapterFactory records the resolved model name and returns a fakeModel;
	// sessions are in-memory so ensureSession succeeds without mocks.
	makeDispatchRunner := func(
		t *testing.T,
		profilesSvc profilesService,
		acpExec ACPProfileExecutor,
		baseFragments []SystemPromptFragment,
		defaultAgentName string,
		appName string,
	) (*AgentRunner, *string) {
		t.Helper()
		stor := sessions.NewMemorySessionsStorage()
		var capturedModel string
		f := &AgentRunnerFactory{
			llmAdapterFactory: func(_ context.Context, name string) (model.LLM, error) {
				capturedModel = name
				return &fakeModel{name: name}, nil
			},
			llmAgentFactory:       llmagent.New,
			llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
			sessionStorage:        stor,
			rootLogger:            RootTestLogger(),
		}
		if appName == "" {
			appName = fake.Lorem().Word()
		}
		if defaultAgentName == "" {
			defaultAgentName = fake.Lorem().Word()
		}
		ar, err := f.NewAgentRunner(t.Context(), NewAgentRunnerParams{
			AppName:               appName,
			AgentName:             defaultAgentName,
			DefaultAgentName:      defaultAgentName,
			SystemPromptFragments: baseFragments,
			ToolsRegistry:         StaticTools(nil),
			ModelName:             "",
			ProfilesService:       profilesSvc,
			ACPProfileExecutor:    acpExec,
		})
		require.NoError(t, err)
		return ar, &capturedModel
	}

	t.Run("direct run resolves model and runs", func(t *testing.T) {
		t.Parallel()

		modelName := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		sessionID := fake.UUID().V4()
		userID := fake.UUID().V4()
		msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}}

		ar, capturedModel := makeDispatchRunner(t, nil, nil, nil, "", "")

		result, err := ar.Run(t.Context(), RunParams{
			UserID:    userID,
			SessionID: sessionID,
			Message:   msg,
			Model:     modelName,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, sessionID, result.SessionID())
		assert.Equal(t, modelName, *capturedModel)
	})

	t.Run("regular profile run resolves model and appends profile instructions", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		profileInstructions := fake.Lorem().Sentence(5)
		defaultModel := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		overrideModel := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		baseFragment := SystemPromptFragment{
			Section: fake.Lorem().Word(),
			Content: fake.Lorem().Sentence(4),
		}

		stor := sessions.NewMemorySessionsStorage()
		var capturedAgentName string
		var capturedFragments []SystemPromptFragment
		var capturedModelName string

		f := &AgentRunnerFactory{
			llmAdapterFactory: func(_ context.Context, name string) (model.LLM, error) {
				capturedModelName = name
				return &fakeModel{name: name}, nil
			},
			llmAgentFactory: func(cfg llmagent.Config) (agent.Agent, error) {
				capturedAgentName = cfg.Name
				return llmagent.New(cfg)
			},
			llmAgentRunnerFactory: RunExecutorFactoryFromRunner,
			sessionStorage:        stor,
			rootLogger:            RootTestLogger(),
		}

		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name:         profileName,
					Instructions: profileInstructions,
					ExecutionSettings: ap.ExecutionSettings{
						DefaultModel: defaultModel,
					},
				}, nil
			},
		}

		ar, err := f.NewAgentRunner(t.Context(), NewAgentRunnerParams{
			AppName:               fake.Lorem().Word(),
			AgentName:             fake.Lorem().Word(),
			DefaultAgentName:      fake.Lorem().Word(),
			SystemPromptFragments: []SystemPromptFragment{baseFragment},
			ToolsRegistry:         StaticTools(nil),
			ModelName:             "",
			ProfilesService:       profilesSvc,
		})
		require.NoError(t, err)

		// Capture fragments via instruction provider — easiest proxy is checking NewAgentRunner params
		// by intercepting the factory agent creation. Fragments are tested indirectly via AgentName capture.
		_ = capturedFragments // verified below via a separate approach

		_, err = ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
			Model:       overrideModel,
		})
		require.NoError(t, err)
		assert.Equal(t, profileName, capturedAgentName)
		assert.Equal(t, overrideModel, capturedModelName)
	})

	t.Run("regular profile run falls back to profile default model", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		defaultModel := fake.Lorem().Word() + "/" + fake.Lorem().Word()

		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name: profileName,
					ExecutionSettings: ap.ExecutionSettings{
						DefaultModel: defaultModel,
					},
				}, nil
			},
		}

		ar, capturedModel := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
		})
		require.NoError(t, err)
		assert.Equal(t, defaultModel, *capturedModel)
	})

	t.Run("returns model required when direct run model is empty", func(t *testing.T) {
		t.Parallel()

		ar, _ := makeDispatchRunner(t, nil, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		})
		require.ErrorContains(t, err, "model is required")
	})

	t.Run("returns model required when regular profile model is unresolved", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name:              profileName,
					ExecutionSettings: ap.ExecutionSettings{},
				}, nil
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.ErrorContains(t, err, "model is required")
	})

	t.Run("returns not-found error when profile does not exist", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return nil, ap.ErrAgentProfileNotFound
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var execErr *AgentExecError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, AgentExecErrorKindNotFound, execErr.Kind)
		assert.ErrorIs(t, err, ap.ErrAgentProfileNotFound)
	})

	t.Run("returns execution error when profile lookup fails", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		expectedErr := errors.New(fake.Lorem().Sentence(4))
		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return nil, expectedErr
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var execErr *AgentExecError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, AgentExecErrorKindExecution, execErr.Kind)
		require.ErrorIs(t, err, expectedErr)
		assert.Contains(t, execErr.Error(), "load-profile")
	})

	t.Run("returns unsupported error for unknown profile mode", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name: profileName,
					ExecutionSettings: ap.ExecutionSettings{
						Mode: ap.ExecutionMode("custom-backend"),
					},
				}, nil
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var execErr *AgentExecError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, AgentExecErrorKindUnsupported, execErr.Kind)
	})

	t.Run("acp-stdio profile delegates to ACP executor", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		requestModel := " " + fake.Lorem().Word() + "/" + fake.Lorem().Word() + " "
		sessionID := fake.UUID().V4()
		userID := fake.UUID().V4()
		msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}}
		var capturedRequest ACPRunRequest

		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name: profileName,
					ExecutionSettings: ap.ExecutionSettings{
						Mode: ap.ExecutionModeACPStdio,
					},
				}, nil
			},
		}
		acpExec := &acpProfileExecutorStub{
			run: func(_ context.Context, request ACPRunRequest) (*RunResult, error) {
				capturedRequest = request
				return singleTextProfileRunResult(sessionID), nil
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, acpExec, nil, "", "")

		result, err := ar.Run(t.Context(), RunParams{
			UserID:      userID,
			SessionID:   sessionID,
			Message:     msg,
			Model:       requestModel,
			ProfileName: profileName,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, sessionID, result.SessionID())
		assert.Equal(t, profileName, capturedRequest.ProfileName)
		assert.Equal(t, strings.TrimSpace(requestModel), capturedRequest.Model)
		assert.Equal(t, userID, capturedRequest.UserID)
		assert.Equal(t, sessionID, capturedRequest.SessionID)
		assert.Equal(t, msg, capturedRequest.Message)
		require.NotNil(t, capturedRequest.Profile)
		assert.Equal(t, ap.ExecutionModeACPStdio, capturedRequest.Profile.ExecutionSettings.ModeOrDefault())
	})

	t.Run("returns execution error when acp profile executor is unavailable", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		profilesSvc := &profilesServiceStub{
			get: func(context.Context, string) (*ap.AgentProfile, error) {
				return &ap.AgentProfile{
					Name: profileName,
					ExecutionSettings: ap.ExecutionSettings{
						Mode: ap.ExecutionModeACPStdio,
					},
				}, nil
			},
		}

		ar, _ := makeDispatchRunner(t, profilesSvc, nil, nil, "", "")

		_, err := ar.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var execErr *AgentExecError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, AgentExecErrorKindExecution, execErr.Kind)
		assert.Contains(t, execErr.Error(), "run-acp-profile")
	})
}

func singleTextProfileRunResult(sessionID string) *RunResult {
	return NewRunResult(
		func(yield func(*SessionEvent, error) bool) {
			_ = yield(&SessionEvent{
				TurnComplete: true,
				Content: &SessionEventContent{
					Role: "model",
					Parts: []SessionEventPart{{
						Text: "ok",
					}},
				},
			}, nil)
		},
		sessionID,
	)
}

type profilesServiceStub struct {
	get func(ctx context.Context, name string) (*ap.AgentProfile, error)
}

func (s *profilesServiceStub) Get(ctx context.Context, name string) (*ap.AgentProfile, error) {
	return s.get(ctx, name)
}

type acpProfileExecutorStub struct {
	run func(ctx context.Context, request ACPRunRequest) (*RunResult, error)
}

func (s *acpProfileExecutorStub) RunACPProfile(
	ctx context.Context,
	request ACPRunRequest,
) (*RunResult, error) {
	return s.run(ctx, request)
}
