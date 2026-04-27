package profileexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

type regularRunner interface {
	Run(ctx context.Context, params rt.RunParams) (*rt.RunResult, error)
}

// ErrorKind classifies profile execution dispatch failures.
type ErrorKind string

const (
	// ErrorKindValidation indicates invalid dispatch input.
	ErrorKindValidation ErrorKind = "validation"
	// ErrorKindNotFound indicates the requested profile does not exist.
	ErrorKindNotFound ErrorKind = "not-found"
	// ErrorKindUnsupported indicates the selected execution mode is not wired yet.
	ErrorKindUnsupported ErrorKind = "unsupported"
	// ErrorKindExecution indicates a lower-level dependency failed during dispatch.
	ErrorKindExecution ErrorKind = "execution"
)

// Error wraps dispatch failures with a stable kind and operation.
type Error struct {
	Kind ErrorKind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("profile execution %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

// RunRequest identifies the profile-backed run to execute.
type RunRequest struct {
	ProfileName string
	UserID      string
	SessionID   string
	Message     *rt.MessageContent
}

// Dispatcher resolves agent profiles and dispatches their execution mode.
type Dispatcher struct {
	profiles      ap.AgentProfilesService
	regularRunner regularRunner
}

// NewDispatcher constructs a profile execution dispatcher.
func NewDispatcher(
	profiles ap.AgentProfilesService,
	regularRunner regularRunner,
) (*Dispatcher, error) {
	if profiles == nil {
		return nil, errors.New("profiles service is required")
	}
	if regularRunner == nil {
		return nil, errors.New("regular runner is required")
	}

	return &Dispatcher{
		profiles:      profiles,
		regularRunner: regularRunner,
	}, nil
}

// Run loads the selected profile and executes it through the configured mode.
func (d *Dispatcher) Run(ctx context.Context, request RunRequest) (*rt.RunResult, error) {
	profileName := strings.TrimSpace(request.ProfileName)
	if profileName == "" {
		return nil, wrapError(ErrorKindValidation, "validate-profile-name", errors.New("profile name is required"))
	}

	profile, err := d.profiles.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, wrapError(
				ErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}

		return nil, wrapError(
			ErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ap.ExecutionModeRegular:
		result, runErr := d.regularRunner.Run(ctx, rt.RunParams{
			UserID:    request.UserID,
			SessionID: request.SessionID,
			Message:   request.Message,
			Model:     profile.ExecutionSettings.DefaultModel,
		})
		if runErr != nil {
			return nil, wrapError(
				ErrorKindExecution,
				"run-regular-profile",
				fmt.Errorf("run profile %q: %w", profile.Name, runErr),
			)
		}

		return result, nil
	case ap.ExecutionModeACPStdio:
		return nil, wrapError(
			ErrorKindUnsupported,
			"dispatch-acp-stdio-profile",
			fmt.Errorf("profile %q uses execution mode %q", profile.Name, ap.ExecutionModeACPStdio),
		)
	default:
		return nil, wrapError(
			ErrorKindUnsupported,
			"dispatch-profile",
			fmt.Errorf("profile %q uses unsupported execution mode %q", profile.Name, profile.ExecutionSettings.Mode),
		)
	}
}

func wrapError(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}

	return &Error{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}
