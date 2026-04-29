package profilerun

import (
	"context"
	"errors"
	"fmt"
	"strings"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// AgentRunner executes built-in runtime runs.
type AgentRunner interface {
	Run(ctx context.Context, params rt.RunParams) (*rt.RunResult, error)
}

// NewAgentRunnerFunc constructs a built-in runner for a resolved run path.
type NewAgentRunnerFunc func(ctx context.Context, params rt.NewAgentRunnerParams) (AgentRunner, error)

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
	Message     *rt.MessageContent
}

// ACPProfileExecutor executes ACP stdio profile runs behind an internal boundary.
type ACPProfileExecutor interface {
	RunACPProfile(ctx context.Context, request ACPRunRequest) (*rt.RunResult, error)
}

// ExecutionRunnerParams configures an internal profile-aware execution runner.
type ExecutionRunnerParams struct {
	NewAgentRunner        NewAgentRunnerFunc
	ToolsProvider         rt.ToolsProvider
	ProfilesService       profilesService
	ACPProfileExecutor    ACPProfileExecutor
	AppName               string
	DefaultAgentName      string
	SystemPromptFragments []rt.SystemPromptFragment
}

// ExecutionRunner owns runtime execution-path selection for direct/profile runs.
type ExecutionRunner struct {
	newAgentRunner        NewAgentRunnerFunc
	toolsProvider         rt.ToolsProvider
	profilesService       profilesService
	acpProfileExecutor    ACPProfileExecutor
	appName               string
	defaultAgentName      string
	systemPromptFragments []rt.SystemPromptFragment
}

func NewExecutionRunner(params ExecutionRunnerParams) *ExecutionRunner {
	toolsProvider := params.ToolsProvider
	if toolsProvider == nil {
		toolsProvider = rt.StaticTools(nil)
	}

	return &ExecutionRunner{
		newAgentRunner:        params.NewAgentRunner,
		toolsProvider:         toolsProvider,
		profilesService:       params.ProfilesService,
		acpProfileExecutor:    params.ACPProfileExecutor,
		appName:               params.AppName,
		defaultAgentName:      params.DefaultAgentName,
		systemPromptFragments: append([]rt.SystemPromptFragment(nil), params.SystemPromptFragments...),
	}
}

func (r *ExecutionRunner) Run(ctx context.Context, params rt.RunParams) (*rt.RunResult, error) {
	profileName := strings.TrimSpace(params.ProfileName)
	requestModel := strings.TrimSpace(params.Model)
	if profileName == "" {
		if requestModel == "" {
			return nil, errors.New("model is required")
		}
		return r.runBuiltInExecution(
			ctx,
			params,
			params.Model,
			r.defaultAgentName,
			"",
		)
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

		return r.runBuiltInExecution(
			ctx,
			params,
			resolvedModel,
			profile.Name,
			profile.Instructions,
		)
	case ap.ExecutionModeACPStdio:
		return r.runACPProfileExecution(ctx, params, profile, requestModel)
	default:
		return nil, WrapError(
			ErrorKindUnsupported,
			"dispatch-profile",
			fmt.Errorf(
				"profile %q uses unsupported execution mode %q",
				profile.Name,
				profile.ExecutionSettings.Mode,
			),
		)
	}
}

func (r *ExecutionRunner) loadProfile(
	ctx context.Context,
	profileName string,
) (*ap.AgentProfile, error) {
	if r.profilesService == nil {
		return nil, WrapError(
			ErrorKindExecution,
			"load-profile",
			errors.New("profile execution unavailable"),
		)
	}

	profile, err := r.profilesService.Get(ctx, profileName)
	if err != nil {
		if errors.Is(err, ap.ErrAgentProfileNotFound) {
			return nil, WrapError(
				ErrorKindNotFound,
				"load-profile",
				fmt.Errorf("profile %q not found: %w", profileName, err),
			)
		}

		return nil, WrapError(
			ErrorKindExecution,
			"load-profile",
			fmt.Errorf("load profile %q: %w", profileName, err),
		)
	}

	return profile, nil
}

func (r *ExecutionRunner) runBuiltInExecution(
	ctx context.Context,
	params rt.RunParams,
	modelName string,
	agentName string,
	profileInstructions string,
) (*rt.RunResult, error) {
	if r.newAgentRunner == nil {
		return nil, errors.New("new agent runner is required")
	}

	agentRunner, err := r.newAgentRunner(
		ctx,
		r.newAgentRunnerParams(modelName, agentName, profileInstructions),
	)
	if err != nil {
		return nil, err
	}

	runParams := rt.RunParams{
		UserID:    params.UserID,
		SessionID: params.SessionID,
		Message:   params.Message,
		Model:     modelName,
	}

	return agentRunner.Run(ctx, runParams)
}

func (r *ExecutionRunner) newAgentRunnerParams(
	modelName string,
	agentName string,
	profileInstructions string,
) rt.NewAgentRunnerParams {
	systemPromptFragments := append(
		[]rt.SystemPromptFragment(nil),
		r.systemPromptFragments...,
	)
	if strings.TrimSpace(profileInstructions) != "" {
		systemPromptFragments = append(systemPromptFragments, rt.SystemPromptFragment{
			Section: "Profile Instructions",
			Content: profileInstructions,
		})
	}

	return rt.NewAgentRunnerParams{
		AppName:               r.appName,
		AgentName:             agentName,
		SystemPromptFragments: systemPromptFragments,
		ToolsRegistry:         r.toolsProvider,
		ModelName:             modelName,
	}
}

func (r *ExecutionRunner) runACPProfileExecution(
	ctx context.Context,
	params rt.RunParams,
	profile *ap.AgentProfile,
	requestModel string,
) (*rt.RunResult, error) {
	if r.acpProfileExecutor == nil {
		return nil, WrapError(
			ErrorKindExecution,
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
