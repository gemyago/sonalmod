package profileexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/gemyago/sonalmod/runtime/internal/profilerun"
)

type acpExecutor interface {
	Execute(ctx context.Context, request codinglane.ACPStdioExecutorRequest) (*codinglane.ACPStdioExecutorResult, error)
}

// RunRequest identifies the profile-backed run to execute.
type RunRequest struct {
	ProfileName string
	Profile     *ap.AgentProfile
	Model       string
	UserID      string
	SessionID   string
	Message     *rt.MessageContent
}

// Dispatcher resolves agent profiles and dispatches ACP stdio profile execution.
type Dispatcher struct {
	profiles        ap.AgentProfilesService
	acpExecutor     acpExecutor
	sessionRecorder SessionRecorder
}

// NewDispatcher constructs a profile execution dispatcher.
func NewDispatcher(
	profiles ap.AgentProfilesService,
	sessionRecorder SessionRecorder,
) (*Dispatcher, error) {
	return newDispatcherWithACPExecutor(
		profiles,
		codinglane.NewACPStdioExecutor(),
		sessionRecorder,
	)
}

func newDispatcherWithACPExecutor(
	profiles ap.AgentProfilesService,
	acpExecutor acpExecutor,
	sessionRecorder SessionRecorder,
) (*Dispatcher, error) {
	if profiles == nil {
		return nil, errors.New("profiles service is required")
	}
	if acpExecutor == nil {
		return nil, errors.New("ACP stdio executor is required")
	}

	return &Dispatcher{
		profiles:        profiles,
		acpExecutor:     acpExecutor,
		sessionRecorder: sessionRecorder,
	}, nil
}

// Run loads the selected profile and executes it through the configured mode.
func (d *Dispatcher) Run(ctx context.Context, request RunRequest) (*rt.RunResult, error) {
	profileName := strings.TrimSpace(request.ProfileName)
	if profileName == "" {
		return nil, profilerun.WrapError(
			profilerun.ErrorKindValidation,
			"validate-profile-name",
			errors.New("profile name is required"),
		)
	}

	profile, err := d.loadProfile(ctx, profileName, request.Profile)
	if err != nil {
		return nil, err
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ap.ExecutionModeRegular:
		return nil, d.unsupportedProfileExecution(profile)
	case ap.ExecutionModeACPStdio:
		return d.runACPProfile(ctx, request, profile)
	default:
		return nil, d.unsupportedProfileExecution(profile)
	}
}

func (d *Dispatcher) loadProfile(
	ctx context.Context,
	profileName string,
	resolvedProfile *ap.AgentProfile,
) (*ap.AgentProfile, error) {
	if resolvedProfile != nil {
		return resolvedProfile, nil
	}

	profile, err := d.profiles.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, profilerun.WrapError(
				profilerun.ErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}

		return nil, profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	return profile, nil
}

func (d *Dispatcher) runACPProfile(
	ctx context.Context,
	request RunRequest,
	profile *ap.AgentProfile,
) (*rt.RunResult, error) {
	acpRequest, mapErr := codinglane.MapACPStdioExecutorRequest(
		*profile,
		messageContentText(request.Message),
	)
	if mapErr != nil {
		return nil, profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"map-acp-stdio-request",
			fmt.Errorf("run profile %q: %w", profile.Name, mapErr),
		)
	}

	acpResult, execErr := d.acpExecutor.Execute(ctx, acpRequest)
	if execErr != nil {
		events := []*rt.SessionEvent{acpStdioErrorSessionEvent(execErr)}
		recordErr := d.recordACPStdioEvents(ctx, profile.Name, request, events)
		if recordErr != nil {
			return nil, recordErr
		}

		return rt.NewRunResult(sessionEventSeq(events), request.SessionID), nil
	}

	events := buildACPStdioSessionEvents(acpResult)
	recordErr := d.recordACPStdioEvents(ctx, profile.Name, request, events)
	if recordErr != nil {
		return nil, recordErr
	}

	return rt.NewRunResult(sessionEventSeq(events), request.SessionID), nil
}

func (d *Dispatcher) unsupportedProfileExecution(profile *ap.AgentProfile) error {
	return profilerun.WrapError(
		profilerun.ErrorKindUnsupported,
		"dispatch-profile",
		fmt.Errorf(
			"profile %q uses unsupported execution mode %q",
			profile.Name,
			profile.ExecutionSettings.Mode,
		),
	)
}

func (d *Dispatcher) recordACPStdioEvents(
	ctx context.Context,
	profileName string,
	request RunRequest,
	events []*rt.SessionEvent,
) error {
	if d.sessionRecorder == nil {
		return nil
	}

	if err := d.sessionRecorder.Record(ctx, request, events); err != nil {
		return profilerun.WrapError(
			profilerun.ErrorKindExecution,
			"record-acp-stdio-session",
			fmt.Errorf("run profile %q: %w", profileName, err),
		)
	}

	return nil
}
