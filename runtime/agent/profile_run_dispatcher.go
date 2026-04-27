package agent

import (
	"context"
	"fmt"

	"github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/profileexec"
)

type MessageContent = internal.MessageContent
type MessagePart = internal.MessagePart

// ProfileRunRequest identifies a profile-backed run to execute.
type ProfileRunRequest struct {
	ProfileName string
	UserID      string
	SessionID   string
	Message     *MessageContent
}

// ProfileRunDispatcher resolves the selected profile and executes it.
type ProfileRunDispatcher interface {
	Run(ctx context.Context, request ProfileRunRequest) (*RunResult, error)
}

type profileRunDispatcher struct {
	dispatcher *profileexec.Dispatcher
}

// NewProfileRunDispatcher creates a profile-aware run dispatcher for standard run endpoints.
func NewProfileRunDispatcher( //nolint:ireturn // public contract returns interface.
	profiles AgentProfilesService,
	regularRunner AgentRunner,
) (ProfileRunDispatcher, error) {
	var sessionRecorder profileexec.SessionRecorder
	if runner, ok := regularRunner.(*Runner); ok {
		recorder, err := profileexec.NewSessionRecorder(defaultRunnerAppName, runner.sessionsStorage)
		if err != nil {
			return nil, fmt.Errorf("create profile run dispatcher: %w", err)
		}
		sessionRecorder = recorder
	}

	dispatcher, err := profileexec.NewDispatcher(profiles, regularRunner, sessionRecorder)
	if err != nil {
		return nil, fmt.Errorf("create profile run dispatcher: %w", err)
	}

	return &profileRunDispatcher{
		dispatcher: dispatcher,
	}, nil
}

func (d *profileRunDispatcher) Run(ctx context.Context, request ProfileRunRequest) (*RunResult, error) {
	return d.dispatcher.Run(ctx, profileexec.RunRequest{
		ProfileName: request.ProfileName,
		UserID:      request.UserID,
		SessionID:   request.SessionID,
		Message:     request.Message,
	})
}
