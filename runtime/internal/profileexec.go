package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ProfileAgentRunner executes built-in runtime runs.
type ProfileAgentRunner interface {
	Run(ctx context.Context, params RunParams) (*RunResult, error)
}

// NewProfileAgentRunnerFunc constructs a built-in runner for a resolved run path.
type NewProfileAgentRunnerFunc func(ctx context.Context, params NewAgentRunnerParams) (ProfileAgentRunner, error)

type profilesService interface {
	Get(ctx context.Context, name string) (*ap.AgentProfile, error)
}

// ACPRunRequest describes a resolved profile run for ACP stdio execution.
type ACPRunRequest struct {
	ProfileName string
	Profile     *ap.AgentProfile
	Model       string
	UserID      string
	SessionID   string
	Message     *MessageContent
}

// ACPProfileExecutor executes ACP stdio profile runs behind an internal boundary.
type ACPProfileExecutor interface {
	RunACPProfile(ctx context.Context, request ACPRunRequest) (*RunResult, error)
}

// ProfileExecutionRunnerParams configures the standard internal profile execution runner.
type ProfileExecutionRunnerParams struct {
	NewAgentRunner        NewProfileAgentRunnerFunc
	ToolsProvider         ToolsProvider
	ProfilesService       profilesService
	ACPProfileExecutor    ACPProfileExecutor
	AppName               string
	DefaultAgentName      string
	SystemPromptFragments []SystemPromptFragment
}

// ProfileExecutionRunner is the standard internal owner of direct and regular
// profile-backed execution. ACP stdio runs are delegated to ACPProfileExecutor.
type ProfileExecutionRunner struct {
	newAgentRunner        NewProfileAgentRunnerFunc
	toolsProvider         ToolsProvider
	profilesService       profilesService
	acpProfileExecutor    ACPProfileExecutor
	appName               string
	defaultAgentName      string
	systemPromptFragments []SystemPromptFragment
}

// NewProfileExecutionRunner creates a ProfileExecutionRunner from the given params.
func NewProfileExecutionRunner(params ProfileExecutionRunnerParams) *ProfileExecutionRunner {
	toolsProvider := params.ToolsProvider
	if toolsProvider == nil {
		toolsProvider = StaticTools(nil)
	}

	return &ProfileExecutionRunner{
		newAgentRunner:        params.NewAgentRunner,
		toolsProvider:         toolsProvider,
		profilesService:       params.ProfilesService,
		acpProfileExecutor:    params.ACPProfileExecutor,
		appName:               params.AppName,
		defaultAgentName:      params.DefaultAgentName,
		systemPromptFragments: append([]SystemPromptFragment(nil), params.SystemPromptFragments...),
	}
}

// Run dispatches the run according to profile selection and execution mode.
// Direct runs (no profileName) and regular profile runs go through the standard
// built-in agent run path. ACP stdio profiles are delegated to ACPProfileExecutor.
func (r *ProfileExecutionRunner) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	profileName := strings.TrimSpace(params.ProfileName)
	requestModel := strings.TrimSpace(params.Model)
	if profileName == "" {
		if requestModel == "" {
			return nil, errors.New("model is required")
		}
		return r.runBuiltIn(ctx, params, params.Model, r.defaultAgentName, "")
	}

	profile, err := r.loadProfile(ctx, profileName)
	if err != nil {
		return nil, err
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ap.ExecutionModeRegular:
		resolvedModel := requestModel
		if resolvedModel == "" {
			resolvedModel = strings.TrimSpace(profile.ExecutionSettings.DefaultModel)
		}
		if resolvedModel == "" {
			return nil, errors.New("model is required")
		}
		return r.runBuiltIn(ctx, params, resolvedModel, profile.Name, profile.Instructions)
	case ap.ExecutionModeACPStdio:
		return r.runACP(ctx, params, profile, requestModel)
	default:
		return nil, WrapAgentExecError(
			AgentExecErrorKindUnsupported,
			"dispatch-profile",
			fmt.Errorf(
				"profile %q uses unsupported execution mode %q",
				profile.Name,
				profile.ExecutionSettings.Mode,
			),
		)
	}
}

func (r *ProfileExecutionRunner) loadProfile(
	ctx context.Context,
	profileName string,
) (*ap.AgentProfile, error) {
	if r.profilesService == nil {
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"load-profile",
			errors.New("profile execution unavailable"),
		)
	}

	profile, err := r.profilesService.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, WrapAgentExecError(
				AgentExecErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	return profile, nil
}

func (r *ProfileExecutionRunner) runBuiltIn(
	ctx context.Context,
	params RunParams,
	modelName string,
	agentName string,
	profileInstructions string,
) (*RunResult, error) {
	if r.newAgentRunner == nil {
		return nil, errors.New("new agent runner is required")
	}

	agentRunner, err := r.newAgentRunner(ctx, r.buildAgentRunnerParams(modelName, agentName, profileInstructions))
	if err != nil {
		return nil, err
	}

	return agentRunner.Run(ctx, RunParams{
		UserID:    params.UserID,
		SessionID: params.SessionID,
		Message:   params.Message,
		Model:     modelName,
	})
}

func (r *ProfileExecutionRunner) buildAgentRunnerParams(
	modelName string,
	agentName string,
	profileInstructions string,
) NewAgentRunnerParams {
	fragments := append([]SystemPromptFragment(nil), r.systemPromptFragments...)
	if strings.TrimSpace(profileInstructions) != "" {
		fragments = append(fragments, SystemPromptFragment{
			Section: "Profile Instructions",
			Content: profileInstructions,
		})
	}
	return NewAgentRunnerParams{
		AppName:               r.appName,
		AgentName:             agentName,
		SystemPromptFragments: fragments,
		ToolsRegistry:         r.toolsProvider,
		ModelName:             modelName,
	}
}

func (r *ProfileExecutionRunner) runACP(
	ctx context.Context,
	params RunParams,
	profile *ap.AgentProfile,
	requestModel string,
) (*RunResult, error) {
	if r.acpProfileExecutor == nil {
		return nil, WrapAgentExecError(
			AgentExecErrorKindExecution,
			"run-acp-profile",
			errors.New("ACP profile runner unavailable"),
		)
	}

	return r.acpProfileExecutor.RunACPProfile(ctx, ACPRunRequest{
		ProfileName: profile.Name,
		Profile:     profile,
		Model:       requestModel,
		UserID:      params.UserID,
		SessionID:   params.SessionID,
		Message:     params.Message,
	})
}
