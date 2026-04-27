package codinglane

import (
	"context"
	"errors"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ACPStdioExecutorRequest defines profile-owned ACP stdio launch input.
type ACPStdioExecutorRequest struct {
	ExecutionSettings agentprofiles.ExecutionSettings
	Prompt            string
	MCPServers        []any
}

// ACPStdioExecutorResult contains session metadata and prompt result.
type ACPStdioExecutorResult = ACPStdioLaunchResult

type acpStdioLaunchClient interface {
	Launch(ctx context.Context, request ACPStdioLaunchRequest) (*ACPStdioLaunchResult, error)
}

// ACPStdioExecutor executes ACP stdio runs from profile execution settings.
type ACPStdioExecutor struct {
	client acpStdioLaunchClient
}

// NewACPStdioExecutor creates an executor backed by the existing ACP stdio client.
func NewACPStdioExecutor() *ACPStdioExecutor {
	return newACPStdioExecutorWithClient(NewOpenCodeACPClient())
}

func newACPStdioExecutorWithClient(client acpStdioLaunchClient) *ACPStdioExecutor {
	return &ACPStdioExecutor{client: client}
}

// Execute launches an ACP stdio run using profile-owned command settings.
func (e *ACPStdioExecutor) Execute(
	ctx context.Context,
	request ACPStdioExecutorRequest,
) (*ACPStdioExecutorResult, error) {
	if request.ExecutionSettings.ModeOrDefault() != agentprofiles.ExecutionModeACPStdio {
		return nil, &ACPStdioError{
			Kind: ACPStdioErrorKindValidation,
			Op:   "validate-execution-settings",
			Err:  errors.New("execution_settings.mode must be acp-stdio"),
		}
	}

	return e.client.Launch(ctx, ACPStdioLaunchRequest{
		AgentCommand: request.ExecutionSettings.AgentCommand,
		CWD:          request.ExecutionSettings.Cwd,
		Prompt:       request.Prompt,
		MCPServers:   request.MCPServers,
	})
}
