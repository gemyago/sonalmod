package profileexec

import (
	"context"
	"errors"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	run func(ctx context.Context, params rt.RunParams) (*rt.RunResult, error)
}

func (r *testRegularRunner) Run(ctx context.Context, params rt.RunParams) (*rt.RunResult, error) {
	return r.run(ctx, params)
}

func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("requires profiles service", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := NewDispatcher(nil, &testRegularRunner{})

		require.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.ErrorContains(t, err, "profiles service is required")
	})

	t.Run("requires regular runner", func(t *testing.T) {
		t.Parallel()

		dispatcher, err := NewDispatcher(&testProfilesService{}, nil)

		require.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.ErrorContains(t, err, "regular runner is required")
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
				run: func(_ context.Context, params rt.RunParams) (*rt.RunResult, error) {
					assert.Equal(t, request.UserID, params.UserID)
					assert.Equal(t, request.SessionID, params.SessionID)
					assert.Equal(t, request.Message, params.Message)
					assert.Equal(t, modelName, params.Model)
					return expectedResult, nil
				},
			},
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
				run: func(_ context.Context, params rt.RunParams) (*rt.RunResult, error) {
					assert.Equal(t, modelName, params.Model)
					return expectedResult, nil
				},
			},
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
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
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
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
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
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
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

		dispatcher, err := NewDispatcher(
			&testProfilesService{
				get: func(_ context.Context, _ string) (*ap.AgentProfile, error) {
					return &ap.AgentProfile{
						Name: request.ProfileName,
						ExecutionSettings: ap.ExecutionSettings{
							Mode: ap.ExecutionModeACPStdio,
						},
					}, nil
				},
			},
			&testRegularRunner{
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
		)
		require.NoError(t, err)

		result, runErr := dispatcher.Run(t.Context(), request)

		require.Error(t, runErr)
		assert.Nil(t, result)
		var dispatchErr *Error
		require.ErrorAs(t, runErr, &dispatchErr)
		assert.Equal(t, ErrorKindUnsupported, dispatchErr.Kind)
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
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					panic("Run should not be called")
				},
			},
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
				run: func(context.Context, rt.RunParams) (*rt.RunResult, error) {
					return nil, expectedErr
				},
			},
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
