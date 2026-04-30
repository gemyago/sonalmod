package acpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdioExecutor(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	makeRequest := func() ExecutorRequest {
		return ExecutorRequest{
			ExecutionSettings: agentprofiles.ExecutionSettings{
				Mode: agentprofiles.ExecutionModeACPStdio,
				AgentCommand: agentprofiles.ACPStdioAgentCommand{
					Command: fake.Lorem().Word(),
					Args:    []string{fake.Lorem().Word(), fake.Lorem().Word()},
				},
				Cwd: "/" + fake.Lorem().Word(),
			},
			Prompt: fake.Lorem().Sentence(6),
			MCPServers: []any{
				map[string]any{"name": fake.Lorem().Word()},
			},
		}
	}

	t.Run("forwards profile execution settings to ACP launch client", func(t *testing.T) {
		t.Parallel()

		request := makeRequest()
		expectedResult := &ExecutorResult{
			SessionID:    fake.UUID().V4(),
			PromptResult: json.RawMessage(`{"ok":true}`),
		}

		type capturedRequest struct {
			value LaunchRequest
		}
		captured := &capturedRequest{}

		executor := newStdioExecutorWithClient(&fakeACPLaunchClient{
			launch: func(_ context.Context, req LaunchRequest) (*LaunchResult, error) {
				captured.value = req
				return expectedResult, nil
			},
		})

		result, err := executor.Execute(t.Context(), request)
		require.NoError(t, err)
		assert.Equal(t, request.ExecutionSettings.AgentCommand.Command, captured.value.AgentCommand.Command)
		assert.Equal(t, request.ExecutionSettings.AgentCommand.Args, captured.value.AgentCommand.Args)
		assert.Equal(t, request.ExecutionSettings.Cwd, captured.value.CWD)
		assert.Equal(t, request.Prompt, captured.value.Prompt)
		assert.Equal(t, request.MCPServers, captured.value.MCPServers)
		assert.Equal(t, expectedResult, result)
	})

	t.Run("rejects non ACP stdio execution settings", func(t *testing.T) {
		t.Parallel()

		executor := newStdioExecutorWithClient(&fakeACPLaunchClient{
			launch: func(context.Context, LaunchRequest) (*LaunchResult, error) {
				t.Fatal("unexpected launch call")
				return nil, errors.New("unexpected launch call")
			},
		})

		_, err := executor.Execute(t.Context(), ExecutorRequest{
			ExecutionSettings: agentprofiles.ExecutionSettings{
				DefaultModel: "openai/gpt-5",
			},
			Prompt: fake.Lorem().Sentence(4),
		})
		require.Error(t, err)

		var execErr *LaunchError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, LaunchErrorKindValidation, execErr.Kind)
		assert.ErrorContains(t, err, "execution_settings.mode must be acp-stdio")
	})

	t.Run("returns launch client errors unchanged", func(t *testing.T) {
		t.Parallel()

		request := makeRequest()
		expectedErr := &LaunchError{
			Kind: LaunchErrorKindProtocol,
			Op:   "session/prompt",
			Err:  errors.New("protocol failed"),
		}

		executor := newStdioExecutorWithClient(&fakeACPLaunchClient{
			launch: func(context.Context, LaunchRequest) (*LaunchResult, error) {
				return nil, expectedErr
			},
		})

		_, err := executor.Execute(t.Context(), request)
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
}

type fakeACPLaunchClient struct {
	launch func(context.Context, LaunchRequest) (*LaunchResult, error)
}

func (f *fakeACPLaunchClient) Launch(
	ctx context.Context,
	request LaunchRequest,
) (*LaunchResult, error) {
	return f.launch(ctx, request)
}
