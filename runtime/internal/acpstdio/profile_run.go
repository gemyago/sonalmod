package acpstdio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/gemyago/sonalmod/runtime/internal/profilerun"
)

// Executor runs ACP stdio requests derived from a resolved profile.
type Executor interface {
	Execute(ctx context.Context, request codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error)
}

// ACPProfileRunner executes ACP stdio profile runs and records their replayable session history.
type ACPProfileRunner struct {
	executor Executor
	recorder SessionRecorder
}

// NewACPProfileRunner creates a runner backed by the default ACP stdio executor.
func NewACPProfileRunner(recorder SessionRecorder) (*ACPProfileRunner, error) {
	return NewACPProfileRunnerWithExecutor(codinglane.NewACPStdioExecutor(), recorder)
}

// NewACPProfileRunnerWithExecutor creates a runner with an injected executor.
func NewACPProfileRunnerWithExecutor(executor Executor, recorder SessionRecorder) (*ACPProfileRunner, error) {
	if executor == nil {
		return nil, errors.New("ACP stdio executor is required")
	}

	return &ACPProfileRunner{
		executor: executor,
		recorder: recorder,
	}, nil
}

// Run executes the resolved ACP stdio profile and returns a standard runtime run result.
func (r *ACPProfileRunner) Run(ctx context.Context, request RunRequest) (*rt.RunResult, error) {
	if request.Profile == nil {
		return nil, profilerun.WrapError(
			profilerun.ErrorKindValidation,
			"run-acp-profile",
			errors.New("profile is required"),
		)
	}

	acpRequest, mapErr := codinglane.MapACPStdioExecutorRequest(
		*request.Profile,
		MessageContentText(request.Message),
	)
	if mapErr != nil {
		return nil, profilerun.WrapError(
			profilerun.ErrorKindExecution,
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
	request RunRequest,
	events []*rt.SessionEvent,
) error {
	if r.recorder == nil {
		return nil
	}

	if err := r.recorder.Record(ctx, request, events); err != nil {
		return profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"record-acp-stdio-session",
			fmt.Errorf("run profile %q: %w", profileRunName(request), err),
		)
	}

	return nil
}

func profileRunName(request RunRequest) string {
	if request.Profile != nil {
		name := strings.TrimSpace(request.Profile.Name)
		if name != "" {
			return name
		}
	}

	return strings.TrimSpace(request.ProfileName)
}
