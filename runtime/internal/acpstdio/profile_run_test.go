package acpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type profileRunnerExecutorStub struct {
	execute func(ctx context.Context, request ExecutorRequest) (*ExecutorResult, error)
}

func (s *profileRunnerExecutorStub) Execute(
	ctx context.Context,
	request ExecutorRequest,
) (*ExecutorResult, error) {
	return s.execute(ctx, request)
}

type profileRunnerRecorderStub struct {
	record func(ctx context.Context, request rt.ACPRunRequest, events []*rt.SessionEvent) error
}

func (r *profileRunnerRecorderStub) Record(
	ctx context.Context,
	request rt.ACPRunRequest,
	events []*rt.SessionEvent,
) error {
	return r.record(ctx, request, events)
}

func TestACPProfileRunner(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	newProfile := func() *ap.AgentProfile {
		return &ap.AgentProfile{
			Name:         fake.Lorem().Word(),
			Instructions: fake.Lorem().Sentence(5),
			ExecutionSettings: ap.ExecutionSettings{
				Mode: ap.ExecutionModeACPStdio,
				AgentCommand: ap.ACPStdioAgentCommand{
					Command: "opencode",
					Args:    []string{"acp"},
				},
			},
		}
	}

	collectEvents := func(t *testing.T, result *rt.RunResult) []*rt.SessionEvent {
		t.Helper()

		events := make([]*rt.SessionEvent, 0)
		for event, err := range result.Events() {
			require.NoError(t, err)
			events = append(events, event)
		}

		return events
	}

	t.Run("requires executor", func(t *testing.T) {
		t.Parallel()

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{})

		require.Error(t, err)
		assert.Nil(t, runner)
		assert.ErrorContains(t, err, "ACP stdio executor is required")
	})

	t.Run("constructs with the default executor", func(t *testing.T) {
		t.Parallel()

		runner, err := NewACPProfileRunner(NewACPProfileRunnerParams{
			AppName:        fake.Lorem().Word(),
			SessionStorage: sessions.NewMemorySessionsStorage(),
		})

		require.NoError(t, err)
		require.NotNil(t, runner)
	})

	t.Run("runs profile and records replayable session events", func(t *testing.T) {
		t.Parallel()

		profile := newProfile()
		messageText := fake.Lorem().Sentence(4)
		request := rt.ACPRunRequest{
			ProfileName: profile.Name,
			Profile:     profile,
			Model:       fake.Lorem().Word(),
			UserID:      fake.Internet().User(),
			SessionID:   fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: messageText}},
			},
		}

		var recordedRequest rt.ACPRunRequest
		var recordedEvents []*rt.SessionEvent

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(_ context.Context, req ExecutorRequest) (*ExecutorResult, error) {
					assert.Equal(t, profile.ExecutionSettings, req.ExecutionSettings)
					assert.Contains(t, req.Prompt, messageText)

					return &ExecutorResult{
						SessionID: fake.UUID().V4(),
						Updates: []Update{
							{
								Type: "progress",
								Payload: json.RawMessage(
									`{"message":"` + fake.Lorem().Sentence(2) + `"}`,
								),
							},
							{
								Type: "final",
								Payload: json.RawMessage(
									`{"message":"` + fake.Lorem().Sentence(2) + `"}`,
								),
							},
						},
					}, nil
				},
			},
			Recorder: &profileRunnerRecorderStub{
				record: func(_ context.Context, req rt.ACPRunRequest, events []*rt.SessionEvent) error {
					recordedRequest = req
					recordedEvents = append(recordedEvents, events...)
					return nil
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), request)

		require.NoError(t, runErr)
		require.NotNil(t, result)
		assert.Equal(t, request.SessionID, result.SessionID())
		assert.Equal(t, request, recordedRequest)

		events := collectEvents(t, result)
		require.Len(t, events, 2)
		require.Len(t, recordedEvents, 2)
		assert.Equal(t, events, recordedEvents)
		assert.True(t, events[0].Partial)
		assert.False(t, events[1].Partial)
		assert.True(t, events[1].TurnComplete)
	})

	t.Run("returns replayable error event when executor fails", func(t *testing.T) {
		t.Parallel()

		profile := newProfile()
		expectedErr := errors.New(fake.Lorem().Sentence(4))

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(context.Context, ExecutorRequest) (*ExecutorResult, error) {
					return nil, expectedErr
				},
			},
			Recorder: &profileRunnerRecorderStub{
				record: func(context.Context, rt.ACPRunRequest, []*rt.SessionEvent) error {
					return nil
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), rt.ACPRunRequest{
			ProfileName: profile.Name,
			Profile:     profile,
			UserID:      fake.Internet().User(),
			SessionID:   fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		})

		require.NoError(t, runErr)
		require.NotNil(t, result)

		events := collectEvents(t, result)
		require.Len(t, events, 1)
		assert.Equal(t, "acp-stdio-execution", events[0].ErrorCode)
		assert.Contains(t, events[0].ErrorMessage, "ACP stdio execution failed")
		assert.Contains(t, events[0].ErrorMessage, expectedErr.Error())
	})

	t.Run("returns recording error when executor also fails", func(t *testing.T) {
		t.Parallel()

		profile := newProfile()
		expectedErr := errors.New(fake.Lorem().Sentence(4))

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(context.Context, ExecutorRequest) (*ExecutorResult, error) {
					return nil, errors.New(fake.Lorem().Sentence(4))
				},
			},
			Recorder: &profileRunnerRecorderStub{
				record: func(context.Context, rt.ACPRunRequest, []*rt.SessionEvent) error {
					return expectedErr
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), rt.ACPRunRequest{
			ProfileName: profile.Name,
			Profile:     profile,
			UserID:      fake.Internet().User(),
			SessionID:   fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		})

		require.Error(t, runErr)
		assert.Nil(t, result)
		var profileErr *rt.AgentExecError
		require.ErrorAs(t, runErr, &profileErr)
		assert.Equal(t, rt.AgentExecErrorKindExecution, profileErr.Kind)
		assert.Contains(t, profileErr.Error(), "record-acp-stdio-session")
		assert.ErrorIs(t, runErr, expectedErr)
	})

	t.Run("returns execution error when recording fails", func(t *testing.T) {
		t.Parallel()

		profile := newProfile()
		expectedErr := errors.New(fake.Lorem().Sentence(4))

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(_ context.Context, _ ExecutorRequest) (*ExecutorResult, error) {
					return &ExecutorResult{
						Updates: []Update{{
							Type: "final",
							Payload: json.RawMessage(
								`{"message":"` + fake.Lorem().Sentence(3) + `"}`,
							),
						}},
					}, nil
				},
			},
			Recorder: &profileRunnerRecorderStub{
				record: func(context.Context, rt.ACPRunRequest, []*rt.SessionEvent) error {
					return expectedErr
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), rt.ACPRunRequest{
			ProfileName: profile.Name,
			Profile:     profile,
			UserID:      fake.Internet().User(),
			SessionID:   fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		})

		require.Error(t, runErr)
		assert.Nil(t, result)
		var profileErr *rt.AgentExecError
		require.ErrorAs(t, runErr, &profileErr)
		assert.Equal(t, rt.AgentExecErrorKindExecution, profileErr.Kind)
		assert.Contains(t, profileErr.Error(), "record-acp-stdio-session")
		assert.ErrorIs(t, runErr, expectedErr)
	})

	t.Run("rejects missing profile", func(t *testing.T) {
		t.Parallel()

		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(context.Context, ExecutorRequest) (*ExecutorResult, error) {
					panic("Execute should not be called")
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), rt.ACPRunRequest{})

		require.Error(t, runErr)
		assert.Nil(t, result)
		var profileErr *rt.AgentExecError
		require.ErrorAs(t, runErr, &profileErr)
		assert.Equal(t, rt.AgentExecErrorKindValidation, profileErr.Kind)
		assert.Contains(t, profileErr.Error(), "run-acp-profile")
	})

	t.Run("uses the request profile name when the resolved profile name is blank", func(t *testing.T) {
		t.Parallel()

		requestProfileName := fake.Lorem().Word()
		runner, err := NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
			Executor: &profileRunnerExecutorStub{
				execute: func(context.Context, ExecutorRequest) (*ExecutorResult, error) {
					panic("Execute should not be called")
				},
			},
		})
		require.NoError(t, err)

		result, runErr := runner.RunACPProfile(t.Context(), rt.ACPRunRequest{
			ProfileName: requestProfileName,
			Profile: &ap.AgentProfile{
				ExecutionSettings: ap.ExecutionSettings{
					Mode: ap.ExecutionModeACPStdio,
					AgentCommand: ap.ACPStdioAgentCommand{
						Command: "opencode",
						Args:    []string{"acp"},
					},
				},
			},
			UserID:    fake.Internet().User(),
			SessionID: fake.UUID().V4(),
			Message: &rt.MessageContent{
				Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}},
			},
		})

		require.Error(t, runErr)
		assert.Nil(t, result)
		var profileErr *rt.AgentExecError
		require.ErrorAs(t, runErr, &profileErr)
		assert.Equal(t, rt.AgentExecErrorKindExecution, profileErr.Kind)
		assert.Contains(t, profileErr.Error(), requestProfileName)
	})
}
