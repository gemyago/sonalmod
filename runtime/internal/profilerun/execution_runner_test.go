package profilerun

import (
	"context"
	"errors"
	"strings"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionRunner(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("direct run delegates through built-in execution", func(t *testing.T) {
		t.Parallel()

		modelName := " " + fake.Lorem().Word() + "/" + fake.Lorem().Word() + " "
		sessionID := fake.UUID().V4()
		userID := fake.UUID().V4()
		msg := &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}}}

		var capturedFactoryParams rt.NewAgentRunnerParams
		var capturedRunParams rt.RunParams

		r := NewExecutionRunner(ExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params rt.NewAgentRunnerParams) (AgentRunner, error) {
				capturedFactoryParams = params
				return &runExecutorStub{
					run: func(_ context.Context, params rt.RunParams) (*rt.RunResult, error) {
						capturedRunParams = params
						return singleTextRunResult(sessionID), nil
					},
				}, nil
			},
			AppName:          fake.Lorem().Word(),
			DefaultAgentName: fake.Lorem().Word(),
		})

		result, err := r.Run(t.Context(), rt.RunParams{
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
		baseFragment := rt.SystemPromptFragment{
			Section: fake.Lorem().Word(),
			Content: fake.Lorem().Sentence(4),
		}

		var capturedFactoryParams rt.NewAgentRunnerParams
		var capturedRunParams rt.RunParams

		r := NewExecutionRunner(ExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params rt.NewAgentRunnerParams) (AgentRunner, error) {
				capturedFactoryParams = params
				return &runExecutorStub{
					run: func(_ context.Context, params rt.RunParams) (*rt.RunResult, error) {
						capturedRunParams = params
						return singleTextRunResult(fake.UUID().V4()), nil
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
			SystemPromptFragments: []rt.SystemPromptFragment{
				baseFragment,
			},
		})

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
			Model:       overrideModel,
		})
		require.NoError(t, err)

		assert.Equal(t, profileName, capturedFactoryParams.AgentName)
		assert.Equal(t, overrideModel, capturedFactoryParams.ModelName)
		require.Len(t, capturedFactoryParams.SystemPromptFragments, 2)
		assert.Equal(t, baseFragment, capturedFactoryParams.SystemPromptFragments[0])
		assert.Equal(t, rt.SystemPromptFragment{
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
		var capturedFactoryParams rt.NewAgentRunnerParams

		r := NewExecutionRunner(ExecutionRunnerParams{
			NewAgentRunner: func(_ context.Context, params rt.NewAgentRunnerParams) (AgentRunner, error) {
				capturedFactoryParams = params
				return &runExecutorStub{
					run: func(_ context.Context, _ rt.RunParams) (*rt.RunResult, error) {
						return singleTextRunResult(fake.UUID().V4()), nil
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

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(4)}}},
			ProfileName: profileName,
		})
		require.NoError(t, err)
		assert.Equal(t, defaultModel, capturedFactoryParams.ModelName)
	})

	t.Run("returns model required when direct run model is empty", func(t *testing.T) {
		t.Parallel()

		r := NewExecutionRunner(ExecutionRunnerParams{})

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "model is required")
	})

	t.Run("returns model required when regular profile model is unresolved", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		r := NewExecutionRunner(ExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name:              profileName,
						ExecutionSettings: ap.ExecutionSettings{},
					}, nil
				},
			},
		})

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "model is required")
	})

	t.Run("returns not-found error when profile does not exist", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		r := NewExecutionRunner(ExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return nil, ap.ErrAgentProfileNotFound
				},
			},
		})

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var profileRunErr *Error
		require.ErrorAs(t, err, &profileRunErr)
		assert.Equal(t, ErrorKindNotFound, profileRunErr.Kind)
		assert.ErrorIs(t, err, ap.ErrAgentProfileNotFound)
	})

	t.Run("returns execution error when profile lookup fails", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		expectedErr := errors.New(fake.Lorem().Sentence(4))
		r := NewExecutionRunner(ExecutionRunnerParams{
			ProfilesService: &profilesServiceStub{
				get: func(context.Context, string) (*ap.AgentProfile, error) {
					return nil, expectedErr
				},
			},
		})

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var profileRunErr *Error
		require.ErrorAs(t, err, &profileRunErr)
		assert.Equal(t, ErrorKindExecution, profileRunErr.Kind)
		require.ErrorIs(t, err, expectedErr)
		assert.Contains(t, profileRunErr.Error(), "load-profile")
	})

	t.Run("returns unsupported error for unknown profile mode", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		r := NewExecutionRunner(ExecutionRunnerParams{
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

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var profileRunErr *Error
		require.ErrorAs(t, err, &profileRunErr)
		assert.Equal(t, ErrorKindUnsupported, profileRunErr.Kind)
	})

	t.Run("acp-stdio profile delegates to ACP executor", func(t *testing.T) {
		t.Parallel()

		profileName := "profile-" + fake.Lorem().Word()
		requestModel := " " + fake.Lorem().Word() + "/" + fake.Lorem().Word() + " "
		sessionID := fake.UUID().V4()
		userID := fake.UUID().V4()
		msg := &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}}
		var capturedRequest ACPRunRequest

		r := NewExecutionRunner(ExecutionRunnerParams{
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
				run: func(_ context.Context, request ACPRunRequest) (*rt.RunResult, error) {
					capturedRequest = request
					return singleTextRunResult(sessionID), nil
				},
			},
		})

		result, err := r.Run(t.Context(), rt.RunParams{
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
		r := NewExecutionRunner(ExecutionRunnerParams{
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

		_, err := r.Run(t.Context(), rt.RunParams{
			UserID:      fake.UUID().V4(),
			SessionID:   fake.UUID().V4(),
			Message:     &rt.MessageContent{Parts: []rt.MessagePart{{Text: fake.Lorem().Sentence(3)}}},
			ProfileName: profileName,
		})
		require.Error(t, err)
		var profileRunErr *Error
		require.ErrorAs(t, err, &profileRunErr)
		assert.Equal(t, ErrorKindExecution, profileRunErr.Kind)
		assert.Contains(t, profileRunErr.Error(), "run-acp-profile")
	})
}

func singleTextRunResult(sessionID string) *rt.RunResult {
	return rt.NewRunResult(
		func(yield func(*rt.SessionEvent, error) bool) {
			_ = yield(&rt.SessionEvent{
				TurnComplete: true,
				Content: &rt.SessionEventContent{
					Role: "model",
					Parts: []rt.SessionEventPart{{
						Text: "ok",
					}},
				},
			}, nil)
		},
		sessionID,
	)
}

type runExecutorStub struct {
	run func(ctx context.Context, params rt.RunParams) (*rt.RunResult, error)
}

func (s *runExecutorStub) Run(ctx context.Context, params rt.RunParams) (*rt.RunResult, error) {
	return s.run(ctx, params)
}

type profilesServiceStub struct {
	get func(ctx context.Context, name string) (*ap.AgentProfile, error)
}

func (s *profilesServiceStub) Get(ctx context.Context, name string) (*ap.AgentProfile, error) {
	return s.get(ctx, name)
}

type acpProfileExecutorStub struct {
	run func(ctx context.Context, request ACPRunRequest) (*rt.RunResult, error)
}

func (s *acpProfileExecutorStub) RunACPProfile(
	ctx context.Context,
	request ACPRunRequest,
) (*rt.RunResult, error) {
	return s.run(ctx, request)
}
