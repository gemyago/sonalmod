package codinglane

import (
	"context"
	"errors"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ACPStdioErrorKind classifies ACP stdio execution failure categories.
type ACPStdioErrorKind = OpenCodeACPErrorKind

const (
	// ACPStdioErrorKindValidation indicates invalid launch input.
	ACPStdioErrorKindValidation = OpenCodeACPErrorKindValidation
	// ACPStdioErrorKindSubprocess indicates subprocess startup or I/O failures.
	ACPStdioErrorKindSubprocess = OpenCodeACPErrorKindSubprocess
	// ACPStdioErrorKindProtocol indicates malformed/invalid ACP protocol responses.
	ACPStdioErrorKindProtocol = OpenCodeACPErrorKindProtocol
)

// ACPStdioError wraps ACP stdio execution failures with a stable kind.
type ACPStdioError = OpenCodeACPError

// ACPStdioExecutorRequest defines profile-owned ACP stdio launch input.
type ACPStdioExecutorRequest struct {
	ExecutionSettings agentprofiles.ExecutionSettings
	Prompt            string
	MCPServers        []any
}

// ACPStdioUpdate contains a parsed session/update notification.
type ACPStdioUpdate = OpenCodeACPUpdate

// ACPStdioExecutorResult contains session metadata and prompt result.
type ACPStdioExecutorResult = OpenCodeACPLaunchResult

type acpStdioLaunchClient interface {
	Launch(ctx context.Context, request OpenCodeACPLaunchRequest) (*OpenCodeACPLaunchResult, error)
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

	return e.client.Launch(ctx, OpenCodeACPLaunchRequest{
		AgentCommand: OpenCodeAgentCommand{
			Command: request.ExecutionSettings.AgentCommand.Command,
			Args:    append([]string(nil), request.ExecutionSettings.AgentCommand.Args...),
		},
		CWD:        request.ExecutionSettings.Cwd,
		Prompt:     request.Prompt,
		MCPServers: request.MCPServers,
	})
}
