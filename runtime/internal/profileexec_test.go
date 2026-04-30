//go:build !release

package internal

import (
	"context"
	"errors"
	"strings"
	"testing"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileExecutionRunner(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("direct run delegates through built-in execution", func(t *testing.T) {
		t.Parallel()

		modelName := " " + fake.Lorem().Word() + "/" + fake.Lorem().Word() + " "
		sessionID := fake.UUID().V4()
		userID := fake.UUID().V4()
		msg := &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}}

		var capturedFactoryParams NewAgentRunnerParams
		var capturedRunParams RunParams

		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params NewAgentRunnerParams) (ProfileAgentRunner, error) {
				capturedFactoryParams = params
				return &profileAgentRunnerStub{
					run: func(_ context.Context, params RunParams) (*RunResult, error) {
						capturedRunParams = params
						return singleTextProfileRunResult(sessionID), nil
					},
				}, nil
			},
			AppName:          fake.Lorem().Word(),
			DefaultAgentName: fake.Lorem().Word(),
		})

		result, err := r.Run(t.Context(), RunParams{
			UserID:    userID,
			SessionID: sessionID,
			Message:   msg,
			Model:     modelName,
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, sessionID, result.SessionID())
		assert.Equal(t, modelName, capturedFactoryParams.ModelName)
		assert.Equal(t, modelName, capturedRunParams.Model)
		assert.Equal(t, userID, capturedRunParams.UserID)
		assert.Equal(t, sessionID, capturedRunParams.SessionID)
		assert.Equal(t, msg, capturedRunParams.Message)
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

		var capturedFactoryParams NewAgentRunnerParams
		var capturedRunParams RunParams

		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params NewAgentRunnerParams) (ProfileAgentRunner, error) {
				capturedFactoryParams = params
				return &profileAgentRunnerStub{
					run: func(_ context.Context, params RunParams) (*RunResult, error) {
						capturedRunParams = params
						return singleTextProfileRunResult(fake.UUID().V4()), nil
					},
				}, nil
			},
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name:         profileName,
						Instructions: profileInstructions,
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: defaultModel,
						},
					}, nil
				},
			},
			AppName:          fake.Lorem().Word(),
			DefaultAgentName: fake.Lorem().Word(),
			SystemPromptFragments: []SystemPromptFragment{
				baseFragment,
			},
		})

		_, err := r.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
			Model:       overrideModel,
		})
		require.NoError(t, err)

		assert.Equal(t, profileName, capturedFactoryParams.AgentName)
		assert.Equal(t, overrideModel, capturedFactoryParams.ModelName)
		require.Len(t, capturedFactoryParams.SystemPromptFragments, 2)
		assert.Equal(t, baseFragment, capturedFactoryParams.SystemPromptFragments[0])
		assert.Equal(t, SystemPromptFragment{
			Section: "Profile Instructions",
			Content: profileInstructions,
		}, capturedFactoryParams.SystemPromptFragments[1])

		assert.Equal(t, overrideModel, capturedRunParams.Model)
		assert.Empty(t, capturedRunParams.ProfileName)
	})

	t.Run("regular profile run falls back to profile default model", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		defaultModel := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		var capturedFactoryParams NewAgentRunnerParams

		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params NewAgentRunnerParams) (ProfileAgentRunner, error) {
				capturedFactoryParams = params
				return &profileAgentRunnerStub{
					run: func(_ context.Context, _ RunParams) (*RunResult, error) {
						return singleTextProfileRunResult(fake.UUID().V4()), nil
					},
				}, nil
			},
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: profileName,
						ExecutionSettings: ap.ExecutionSettings{
							DefaultModel: defaultModel,
						},
					}, nil
				},
			},
			AppName:          fake.Lorem().Word(),
			DefaultAgentName: fake.Lorem().Word(),
		})

		_, err := r.Run(t.Context(), RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
		})
		require.NoError(t, err)
		assert.Equal(t, defaultModel, capturedFactoryParams.ModelName)
	})

	t.Run("returns model required when direct run model is empty", func(t *testing.T) {
		t.Parallel()

		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{})

		_, err := r.Run(t.Context(), RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		})
		require.ErrorContains(t, err, "model is required")
	})

	t.Run("returns model required when regular profile model is unresolved", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name:              profileName,
						ExecutionSettings: ap.ExecutionSettings{},
					}, nil
				},
			},
		})

		_, err := r.Run(t.Context(), RunParams{
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
		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return nil, ap.ErrAgentProfileNotFound
				},
			},
		})

		_, err := r.Run(t.Context(), RunParams{
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
		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return nil, expectedErr
				},
			},
		})

		_, err := r.Run(t.Context(), RunParams{
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
		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: profileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionMode("custom-backend"),
						},
					}, nil
				},
			},
		})

		_, err := r.Run(t.Context(), RunParams{
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

		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: profileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
						},
					}, nil
				},
			},
			ACPProfileExecutor: &acpProfileExecutorStub{
				run: func(_ context.Context, request ACPRunRequest) (*RunResult, error) {
					capturedRequest = request
					return singleTextProfileRunResult(sessionID), nil
				},
			},
		})

		result, err := r.Run(t.Context(), RunParams{
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
		r := NewProfileExecutionRunner(ProfileExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: profileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
						},
					}, nil
				},
			},
		})

		_, err := r.Run(t.Context(), RunParams{
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

type profileAgentRunnerStub struct {
	run func(ctx context.Context, params RunParams) (*RunResult, error)
}

func (s *profileAgentRunnerStub) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	return s.run(ctx, params)
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
