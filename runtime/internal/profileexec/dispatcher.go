package profileexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/acpstdio"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/gemyago/sonalmod/runtime/internal/profilerun"
)

// Dispatcher resolves agent profiles and dispatches ACP stdio profile execution.
type Dispatcher struct {
	profiles  ap.AgentProfilesService
	acpRunner *acpstdio.ACPProfileRunner
}

// NewDispatcher constructs a profile execution dispatcher.
func NewDispatcher(
	profiles ap.AgentProfilesService,
	sessionRecorder acpstdio.SessionRecorder,
) (*Dispatcher, error) {
	return newDispatcherWithACPExecutor(
		profiles,
		codinglane.NewACPStdioExecutor(),
		sessionRecorder,
	)
}

func newDispatcherWithACPExecutor(
	profiles ap.AgentProfilesService,
	acpExecutor acpstdio.Executor,
	sessionRecorder acpstdio.SessionRecorder,
) (*Dispatcher, error) {
	if profiles == nil {
		return nil, errors.New("profiles service is required")
	}

	acpRunner, err := acpstdio.NewACPProfileRunnerWithExecutor(acpExecutor, sessionRecorder)
	if err != nil {
		return nil, err
	}
	return &Dispatcher{
		profiles:  profiles,
		acpRunner: acpRunner,
	}, nil
}

// Run loads the selected profile and executes it through the configured mode.
func (d *Dispatcher) Run(ctx context.Context, request acpstdio.RunRequest) (*rt.RunResult, error) {
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
	request acpstdio.RunRequest,
	profile *ap.AgentProfile,
) (*rt.RunResult, error) {
	request.Profile = profile
	if strings.TrimSpace(request.ProfileName) == "" {
		request.ProfileName = profile.Name
	}

	return d.acpRunner.Run(ctx, request)
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
