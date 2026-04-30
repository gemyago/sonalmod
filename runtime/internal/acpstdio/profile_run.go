package acpstdio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
)

// Executor runs ACP stdio requests derived from a resolved profile.
type Executor interface {
	Execute(ctx context.Context, request ExecutorRequest) (*ExecutorResult, error)
}

// ACPProfileRunner executes ACP stdio profile runs and records their replayable session history.
type ACPProfileRunner struct {
	executor Executor
	recorder SessionRecorder
}

// NewACPProfileRunnerParams configures the default ACP profile runner construction.
type NewACPProfileRunnerParams struct {
	AppName        string
	SessionStorage sessionService
}

// NewACPProfileRunnerWithExecutorParams configures ACP profile runner construction
// when an executor and/or recorder is provided by the caller.
type NewACPProfileRunnerWithExecutorParams struct {
	Executor Executor
	Recorder SessionRecorder
}

// NewACPProfileRunner creates a runner backed by the default ACP stdio executor.
func NewACPProfileRunner(params NewACPProfileRunnerParams) (*ACPProfileRunner, error) {
	recorder, err := NewSessionRecorder(params.AppName, params.SessionStorage)
	if err != nil {
		return nil, fmt.Errorf("session recorder: %w", err)
	}

	return NewACPProfileRunnerWithExecutor(NewACPProfileRunnerWithExecutorParams{
		Executor: NewStdioExecutor(),
		Recorder: recorder,
	})
}

// NewACPProfileRunnerWithExecutor creates a runner with an injected executor.
func NewACPProfileRunnerWithExecutor(params NewACPProfileRunnerWithExecutorParams) (*ACPProfileRunner, error) {
	if params.Executor == nil {
		return nil, errors.New("ACP stdio executor is required")
	}

	return &ACPProfileRunner{
		executor: params.Executor,
		recorder: params.Recorder,
	}, nil
}

// RunACPProfile executes the resolved ACP stdio profile and returns a standard runtime run result.
func (r *ACPProfileRunner) RunACPProfile(ctx context.Context, request rt.ACPRunRequest) (*rt.RunResult, error) {
	if request.Profile == nil {
		return nil, rt.WrapAgentExecError(
			rt.AgentExecErrorKindValidation,
			"run-acp-profile",
			errors.New("profile is required"),
		)
	}

	acpRequest, mapErr := MapExecutorRequest(
		*request.Profile,
		MessageContentText(request.Message),
	)
	if mapErr != nil {
		return nil, rt.WrapAgentExecError(
			rt.AgentExecErrorKindExecution,
			"map-acp-stdio-request",
			fmt.Errorf("run profile %q: %w", profileRunName(request), mapErr),
		)
	}

	acpResult, execErr := r.executor.Execute(ctx, acpRequest)
	if execErr != nil {
		events := []*rt.SessionEvent{ErrorSessionEvent(execErr)}
		if recordErr := r.recordEvents(ctx, request, events); recordErr != nil {
			return nil, recordErr
		}

		return rt.NewRunResult(sessionEventSeq(events), request.SessionID), nil
	}

	events := BuildSessionEvents(acpResult)
	if recordErr := r.recordEvents(ctx, request, events); recordErr != nil {
		return nil, recordErr
	}

	return NewRunResult(request.SessionID, acpResult), nil
}

func (r *ACPProfileRunner) recordEvents(
	ctx context.Context,
	request rt.ACPRunRequest,
	events []*rt.SessionEvent,
) error {
	if r.recorder == nil {
		return nil
	}

	if err := r.recorder.Record(ctx, request, events); err != nil {
		return rt.WrapAgentExecError(
			rt.AgentExecErrorKindExecution,
			"record-acp-stdio-session",
			fmt.Errorf("run profile %q: %w", profileRunName(request), err),
		)
	}

	return nil
}

func profileRunName(request rt.ACPRunRequest) string {
	if request.Profile != nil {
		name := strings.TrimSpace(request.Profile.Name)
		if name != "" {
			return name
		}
	}

	return strings.TrimSpace(request.ProfileName)
}
