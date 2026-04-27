package profileexec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
)

type testProfilesService struct {
	get func(ctx context.Context, name string) (*ap.AgentProfile, error)
}

func (s *testProfilesService) List(context.Context) ([]ap.AgentProfile, error) {
	panic("unexpected List call")
}

func (s *testProfilesService) Get(ctx context.Context, name string) (*ap.AgentProfile, error) {
	return s.get(ctx, name)
}

func (s *testProfilesService) Create(
	context.Context,
	ap.CreateAgentProfileParams,
) (*ap.AgentProfile, error) {
	panic("unexpected Create call")
}

func (s *testProfilesService) Update(
	context.Context,
	string,
	ap.UpdateAgentProfileParams,
) (*ap.AgentProfile, error) {
	panic("unexpected Update call")
}

func (s *testProfilesService) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func (s *testProfilesService) AutoMigrate() error {
	panic("unexpected AutoMigrate call")
}

type testRegularRunner struct {
	run func(ctx context.Context, request RegularRunRequest) (*rt.RunResult, error)
}

func (r *testRegularRunner) RunRegularProfile(
	ctx context.Context,
	request RegularRunRequest,
) (*rt.RunResult, error) {
	return r.run(ctx, request)
}

type testACPExecutor struct {
	execute func(ctx context.Context, request codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error)
}

func (e *testACPExecutor) Execute(
	ctx context.Context,
	request codinglane.ACPStdioExecutorRequest,
) (*codinglane.ACPStdioExecutorResult, error) {
	return e.execute(ctx, request)
}

func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("requires profiles service", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := NewDispatcher(nil, &testRegularRunner{}, nil)

		require.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.ErrorContains(t, err, "profiles service is required")
	})

	t.Run("requires regular runner", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := NewDispatcher(&testProfilesService{}, nil, nil)

		require.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.ErrorContains(t, err, "regular runner is required")
	})

	t.Run("requires ACP stdio executor", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := newDispatcherWithACPExecutor(
			&testProfilesService{},
			&testRegularRunner{},
			nil,
			nil,
		)

		require.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.ErrorContains(t, err, "ACP stdio executor is required")
	})
}

func TestDispatcherRun(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newRequest := func() RunRequest {
		return RunRequest{
			ProfileName: fake.Lorem().Word(),
			UserID:      fake.Internet().User(),
			SessionID:   fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		}
	}

	makeResult := func(sessionID string) *rt.RunResult {
		return rt.NewRunResult(func(func(*rt.SessionEvent, error) bool) {}, sessionID)
	}

	t.Run("regular profile dispatches with default model", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		modelName := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		expectedResult := makeResult(request.SessionID)

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, name string) (*ap.AgentProfile, error) {
					assert.Equal(t, request.ProfileName, name)
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode:         ap.ExecutionModeRegular,
							DefaultModel: modelName,
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(_ context.Context, got RegularRunRequest) (*rt.RunResult, error) {
					assert.Equal(t, request.UserID, got.UserID)
					assert.Equal(t, request.SessionID, got.SessionID)
					assert.Equal(t, request.Message, got.Message)
					assert.Equal(t, modelName, got.Model)
					assert.Equal(t, request.ProfileName, got.AgentName)
					return expectedResult, nil
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		assert.Same(t, expectedResult, result)
	})

	t.Run("omitted mode defaults to regular dispatch", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		modelName := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		expectedResult := makeResult(request.SessionID)

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: modelName,
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(_ context.Context, got RegularRunRequest) (*rt.RunResult, error) {
					assert.Equal(t, modelName, got.Model)
					return expectedResult, nil
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		assert.Same(t, expectedResult, result)
	})

	t.Run("request model overrides regular profile default model", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		request.Model = fake.Lorem().Word() + "/" + fake.Lorem().Word()
		defaultModel := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		expectedResult := makeResult(request.SessionID)

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: defaultModel,
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(_ context.Context, got RegularRunRequest) (*rt.RunResult, error) {
					assert.Equal(t, request.Model, got.Model)
					return expectedResult, nil
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		assert.Same(t, expectedResult, result)
	})

	t.Run("missing profile name returns validation error", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					panic("Get should not be called")
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), RunRequest{})

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindValidation, dispatchErr.Kind)
	})

	t.Run("unknown profile returns not found error", func(t *testing.T) {
		t.Parallel()

		request := newRequest()

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return nil, ap.ErrAgentProfileNotFound
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindNotFound, dispatchErr.Kind)
		assert.ErrorIs(t, runErr, ap.ErrAgentProfileNotFound)
	})

	t.Run("profile lookup failure returns execution error", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		expectedErr := errors.New(fake.Lorem().Sentence(4))

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return nil, expectedErr
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindExecution, dispatchErr.Kind)
		require.ErrorIs(t, runErr, expectedErr)
		assert.Contains(t, dispatchErr.Error(), "load-profile")
	})

	t.Run("acp stdio profile returns unsupported error", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		progressText := fake.Lorem().Sentence(3)
		finalText := fake.Lorem().Sentence(4)

		dispatcher, err := newDispatcherWithACPExecutor(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name:         request.ProfileName,
						Role:         "coding-agent",
						Instructions: fake.Lorem().Sentence(5),
						ToolRefs: []string{
							fake.Lorem().Word(),
							fake.Lorem().Word(),
						},
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
							AgentCommand: ap.ACPStdioAgentCommand{
								Command: "opencode",
								Args:    []string{"acp"},
							},
							Cwd: "/workspace",
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			&testACPExecutor{
				execute: func(_ context.Context, req codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error) {
					assert.Equal(t, ap.ExecutionModeACPStdio, req.ExecutionSettings.ModeOrDefault())
					assert.Contains(t, req.Prompt, request.Message.Parts[0].Text)

					return &codinglane.ACPStdioExecutorResult{
						SessionID: fake.UUID().V4(),
						PromptResult: json.RawMessage(
							`{"message":"` + fake.Lorem().Sentence(2) + `"}`,
						),
						Updates: []codinglane.ACPStdioUpdate{
							{
								SessionID: fake.UUID().V4(),
								Type:      "progress",
								Payload: json.RawMessage(
									`{"type":"progress","message":"` + progressText + `"}`,
								),
							},
							{
								SessionID: fake.UUID().V4(),
								Type:      "final",
								Payload: json.RawMessage(
									`{"type":"final","message":"` + finalText + `"}`,
								),
							},
						},
					}, nil
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		require.NotNil(t, result)
		assert.Equal(t, request.SessionID, result.SessionID())

		events := collectSessionEvents(t, result.Events())
		require.Len(t, events, 2)
		assert.True(t, events[0].Partial)
		assert.False(t, events[0].TurnComplete)
		require.NotNil(t, events[0].Content)
		assert.Equal(t, "model", events[0].Content.Role)
		require.Len(t, events[0].Content.Parts, 1)
		assert.Equal(t, progressText, events[0].Content.Parts[0].Text)

		assert.False(t, events[1].Partial)
		assert.True(t, events[1].TurnComplete)
		require.NotNil(t, events[1].Content)
		require.Len(t, events[1].Content.Parts, 1)
		assert.Equal(t, finalText, events[1].Content.Parts[0].Text)
	})

	t.Run("acp stdio execution failure returns stream error event", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		expectedErr := &codinglane.ACPStdioError{
			Kind: codinglane.ACPStdioErrorKindSubprocess,
			Op:   "start-subprocess",
			Err:  errors.New(fake.Lorem().Sentence(4)),
		}

		dispatcher, err := newDispatcherWithACPExecutor(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
							AgentCommand: ap.ACPStdioAgentCommand{
								Command: "opencode",
							},
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			&testACPExecutor{
				execute: func(context.Context, codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error) {
					return nil, expectedErr
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		require.NotNil(t, result)
		assert.Equal(t, request.SessionID, result.SessionID())

		events := collectSessionEvents(t, result.Events())
		require.Len(t, events, 1)
		assert.Equal(t, "acp-stdio-subprocess", events[0].ErrorCode)
		assert.Contains(t, events[0].ErrorMessage, "ACP stdio agent failed to start")
		assert.Contains(t, events[0].ErrorMessage, expectedErr.Err.Error())
	})

	t.Run("unknown execution mode returns unsupported error", func(t *testing.T) {
		t.Parallel()

		request := newRequest()

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionMode("custom-backend"),
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindUnsupported, dispatchErr.Kind)
		assert.Contains(t, dispatchErr.Error(), "dispatch-profile")
	})

	t.Run("runner failure returns execution error", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		modelName := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		expectedErr := errors.New(fake.Lorem().Sentence(4))

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: modelName,
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					return nil, expectedErr
				},
			},
			nil,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindExecution, dispatchErr.Kind)
		assert.ErrorIs(t, runErr, expectedErr)
	})

	t.Run("acp stdio success persists replayable session history", func(t *testing.T) {
		t.Parallel()

		request := newRequest()
		progressText := fake.Lorem().Sentence(3)
		finalText := fake.Lorem().Sentence(4)
		appName := "app-" + fake.Lorem().Word()
		storage := sessions.NewMemorySessionsStorage()
		recorder, err := NewSessionRecorder(appName, storage)
		require.NoError(t, err)

		dispatcher, err := newDispatcherWithACPExecutor(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
							AgentCommand: ap.ACPStdioAgentCommand{
								Command: "opencode",
								Args:    []string{"acp"},
							},
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, RegularRunRequest) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
			&testACPExecutor{
				execute: func(context.Context, codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error) {
					return &codinglane.ACPStdioExecutorResult{
						Updates: []codinglane.ACPStdioUpdate{
							{
								Type: "progress",
								Payload: json.RawMessage(
									`{"message":"` + progressText + `"}`,
								),
							},
							{
								Type: "final",
								Payload: json.RawMessage(
									`{"message":"` + finalText + `"}`,
								),
							},
						},
					}, nil
				},
			},
			recorder,
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.NoError(t, runErr)
		require.NotNil(t, result)
		assert.Equal(t, request.SessionID, result.SessionID())

		streamedEvents := collectSessionEvents(t, result.Events())
		require.Len(t, streamedEvents, 2)
		assert.True(t, streamedEvents[0].Partial)
		assert.True(t, streamedEvents[1].TurnComplete)

		stored, err := storage.Get(t.Context(), &session.GetRequest{
			AppName:   appName,
			UserID:    request.UserID,
			SessionID: request.SessionID,
		})
		require.NoError(t, err)
		require.NotNil(t, stored)
		require.NotNil(t, stored.Session)
		require.Equal(t, 2, stored.Session.Events().Len())

		userEvent := rt.MapADKSessionEvent(stored.Session.Events().At(0))
		require.NotNil(t, userEvent)
		require.NotNil(t, userEvent.Content)
		assert.Equal(t, "user", userEvent.Content.Role)
		require.Len(t, userEvent.Content.Parts, 1)
		assert.Equal(t, request.Message.Parts[0].Text, userEvent.Content.Parts[0].Text)

		finalEvent := rt.MapADKSessionEvent(stored.Session.Events().At(1))
		require.NotNil(t, finalEvent)
		require.NotNil(t, finalEvent.Content)
		assert.True(t, finalEvent.TurnComplete)
		require.Len(t, finalEvent.Content.Parts, 1)
		assert.Equal(t, finalText, finalEvent.Content.Parts[0].Text)
	})
}

func collectSessionEvents(t *testing.T, seq func(func(*rt.SessionEvent, error) bool)) []*rt.SessionEvent {
	t.Helper()

	events := make([]*rt.SessionEvent, 0)
	for event, err := range seq {
		require.NoError(t, err)
		events = append(events, event)
	}

	return events
}

func TestWrapError(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when error is nil", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, wrapError(ErrorKindExecution, "noop", nil))
	})

	t.Run("wraps kind operation and original error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("boom")

		err := wrapError(ErrorKindExecution, "run", expectedErr)

		var dispatchErr *Error
		require.ErrorAs(t, err, &dispatchErr)
		assert.Equal(t, ErrorKindExecution, dispatchErr.Kind)
		assert.Equal(t, "run", dispatchErr.Op)
		require.ErrorIs(t, err, expectedErr)
		assert.Contains(t, err.Error(), "profile execution run (execution)")
	})
}
