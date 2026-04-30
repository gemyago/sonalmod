package acpstdio

import (
	"context"
	"errors"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ExecutorRequest defines profile-owned ACP stdio launch input.
type ExecutorRequest struct {
	ExecutionSettings agentprofiles.ExecutionSettings
	Prompt            string
	MCPServers        []any
}

// ExecutorResult contains session metadata and prompt result.
type ExecutorResult = LaunchResult

type acpLaunchClient interface {
	Launch(ctx context.Context, request LaunchRequest) (*LaunchResult, error)
}

// StdioExecutor executes ACP stdio runs from profile execution settings.
type StdioExecutor struct {
	client acpLaunchClient
}

// NewStdioExecutor creates an executor backed by the default ACP stdio client.
func NewStdioExecutor() *StdioExecutor {
	return newStdioExecutorWithClient(NewOpenCodeACPClient())
}

func newStdioExecutorWithClient(client acpLaunchClient) *StdioExecutor {
	return &StdioExecutor{client: client}
}

// Execute launches an ACP stdio run using profile-owned command settings.
func (e *StdioExecutor) Execute(
	ctx context.Context,
	request ExecutorRequest,
) (*ExecutorResult, error) {
	if request.ExecutionSettings.ModeOrDefault() != agentprofiles.ExecutionModeACPStdio {
		return nil, &LaunchError{
			Kind: LaunchErrorKindValidation,
			Op:   "validate-execution-settings",
			Err:  errors.New("execution_settings.mode must be acp-stdio"),
		}
	}

	return e.client.Launch(ctx, LaunchRequest{
		AgentCommand: request.ExecutionSettings.AgentCommand,
		CWD:          request.ExecutionSettings.Cwd,
		Prompt:       request.Prompt,
		MCPServers:   request.MCPServers,
	})
}
