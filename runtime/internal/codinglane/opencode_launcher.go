package codinglane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// OpenCodeLaunchErrorKind classifies launcher failures for API-level mapping.
type OpenCodeLaunchErrorKind string

const (
	// OpenCodeLaunchErrorKindValidation indicates request/config validation failure.
	OpenCodeLaunchErrorKindValidation OpenCodeLaunchErrorKind = "validation"
	// OpenCodeLaunchErrorKindNotFound indicates missing profile or binding state.
	OpenCodeLaunchErrorKindNotFound OpenCodeLaunchErrorKind = "not-found"
	// OpenCodeLaunchErrorKindLaunchFailed indicates subprocess/protocol execution failure.
	OpenCodeLaunchErrorKindLaunchFailed OpenCodeLaunchErrorKind = "launch-failed"
)

// OpenCodeLaunchError wraps launcher failures with a stable kind and operation.
type OpenCodeLaunchError struct {
	Kind OpenCodeLaunchErrorKind
	Op   string
	Err  error
}

func (e *OpenCodeLaunchError) Error() string {
	return fmt.Sprintf("opencode launch %s (%s): %v", e.Op, e.Kind, e.Err)
}

func (e *OpenCodeLaunchError) Unwrap() error {
	return e.Err
}

// OpenCodeLaunchResult contains resolved launch identifiers and ACP output.
type OpenCodeLaunchResult struct {
	ProfileName  string
	BindingName  string
	SessionID    string
	PromptResult json.RawMessage
	Updates      []OpenCodeACPUpdate
}

type acpStdioExecutor interface {
	Execute(ctx context.Context, request ACPStdioExecutorRequest) (*ACPStdioExecutorResult, error)
}

// OpenCodeACPLauncher resolves saved profile and binding configuration and runs ACP launches.
type OpenCodeACPLauncher struct {
	profiles    agentprofiles.AgentProfilesService
	bindings    OpenCodeBindingService
	acpExecutor acpStdioExecutor
}

// NewOpenCodeACPLauncher constructs a launcher from required dependencies.
func NewOpenCodeACPLauncher(
	profiles agentprofiles.AgentProfilesService,
	bindings OpenCodeBindingService,
	acpExecutor acpStdioExecutor,
) (*OpenCodeACPLauncher, error) {
	if profiles == nil {
		return nil, errors.New("profiles service is required")
	}
	if bindings == nil {
		return nil, errors.New("bindings service is required")
	}
	if acpExecutor == nil {
		return nil, errors.New("ACP stdio executor is required")
	}
	return &OpenCodeACPLauncher{
		profiles:    profiles,
		bindings:    bindings,
		acpExecutor: acpExecutor,
	}, nil
}

func wrapOpenCodeLaunchError(kind OpenCodeLaunchErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &OpenCodeLaunchError{
		Kind: kind,
		Op:   op,
		Err:  err,
	}
}

// Launch resolves saved profile/binding configuration and performs ACP launch.
func (l *OpenCodeACPLauncher) Launch(
	ctx context.Context,
	request OpenCodeLaunchRequest,
) (*OpenCodeLaunchResult, error) {
	profileName := strings.TrimSpace(request.ProfileName)
	if profileName == "" {
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindValidation,
			"validate-profile-name",
			errors.New("profile_name is required"),
		)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindValidation,
			"validate-prompt",
			errors.New("prompt is required"),
		)
	}

	profile, err := l.profiles.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, agentprofiles.ErrAgentProfileNotFound) {
			return nil, wrapOpenCodeLaunchError(
				OpenCodeLaunchErrorKindNotFound,
				"resolve-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindLaunchFailed,
			"resolve-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	binding, err := l.resolveBindingForLaunch(ctx, request, profileName)
	if err != nil {
		return nil, err
	}

	acpRequest, err := MapOpenCodeLaunchRequest(*profile, *binding, request)
	if err != nil {
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindValidation,
			"map-launch-request",
			err,
		)
	}

	acpResult, err := l.acpExecutor.Execute(ctx, acpRequest)
	if err != nil {
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindLaunchFailed,
			"launch-acp-session",
			fmt.Errorf(
				"binding=%s profile=%s: %w",
				binding.Name,
				profile.Name,
				err,
			),
		)
	}

	return &OpenCodeLaunchResult{
		ProfileName:  profile.Name,
		BindingName:  binding.Name,
		SessionID:    acpResult.SessionID,
		PromptResult: acpResult.PromptResult,
		Updates:      acpResult.Updates,
	}, nil
}

func (l *OpenCodeACPLauncher) resolveBindingForLaunch(
	ctx context.Context,
	request OpenCodeLaunchRequest,
	profileName string,
) (*OpenCodeBinding, error) {
	bindingName := strings.TrimSpace(request.BindingName)
	if bindingName != "" {
		binding, err := l.bindings.Get(ctx, bindingName)
		if err != nil {
			if errors.Is(err, ErrOpenCodeBindingNotFound) {
				return nil, wrapOpenCodeLaunchError(
					OpenCodeLaunchErrorKindNotFound,
					"resolve-binding",
					fmt.Errorf("binding %q not found: %w", bindingName, err),
				)
			}
			return nil, wrapOpenCodeLaunchError(
				OpenCodeLaunchErrorKindLaunchFailed,
				"resolve-binding",
				fmt.Errorf("load binding %q: %w", bindingName, err),
			)
		}
		if binding.ProfileName != profileName {
			return nil, wrapOpenCodeLaunchError(
				OpenCodeLaunchErrorKindNotFound,
				"resolve-binding",
				fmt.Errorf(
					"binding %q belongs to profile %q, not %q",
					bindingName,
					binding.ProfileName,
					profileName,
				),
			)
		}
		return binding, nil
	}

	bindings, err := l.bindings.List(ctx)
	if err != nil {
		return nil, wrapOpenCodeLaunchError(
			OpenCodeLaunchErrorKindLaunchFailed,
			"resolve-binding",
			fmt.Errorf("list bindings: %w", err),
		)
	}
	for _, binding := range bindings {
		if binding.ProfileName == profileName {
			copied := binding
			return &copied, nil
		}
	}

	return nil, wrapOpenCodeLaunchError(
		OpenCodeLaunchErrorKindNotFound,
		"resolve-binding",
		fmt.Errorf("binding for profile %q not found", profileName),
	)
}
