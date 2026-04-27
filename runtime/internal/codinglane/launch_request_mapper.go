package codinglane

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// OpenCodeLaunchRequest identifies saved config and a prompt for a coding run.
type OpenCodeLaunchRequest struct {
	ProfileName string
	BindingName string
	Prompt      string
}

// MapACPStdioExecutorRequest composes ACP stdio executor input from profile defaults.
func MapACPStdioExecutorRequest(
	profile agentprofiles.AgentProfile,
	prompt string,
) (ACPStdioExecutorRequest, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ACPStdioExecutorRequest{}, errors.New("prompt is required")
	}

	if profile.Name == "" {
		return ACPStdioExecutorRequest{}, errors.New("profile name is required")
	}
	if profile.ExecutionSettings.ModeOrDefault() != agentprofiles.ExecutionModeACPStdio {
		return ACPStdioExecutorRequest{}, fmt.Errorf(
			"profile %s does not use acp-stdio execution settings",
			profile.Name,
		)
	}

	toolRefs := "none"
	if len(profile.ToolRefs) > 0 {
		toolRefs = strings.Join(profile.ToolRefs, ", ")
	}

	composedPrompt := fmt.Sprintf(
		"Role: %s\nInstructions: %s\nDefault model: %s\nTools: %s\n\nUser prompt:\n%s",
		profile.Role,
		profile.Instructions,
		profile.ExecutionSettings.DefaultModel,
		toolRefs,
		prompt,
	)

	return ACPStdioExecutorRequest{
		ExecutionSettings: profile.ExecutionSettings,
		Prompt:            composedPrompt,
		MCPServers:        []any{},
	}, nil
}

// MapOpenCodeLaunchRequest composes ACP stdio executor input from profile settings.
func MapOpenCodeLaunchRequest(
	profile agentprofiles.AgentProfile,
	binding OpenCodeBinding,
	request OpenCodeLaunchRequest,
) (ACPStdioExecutorRequest, error) {
	if profile.Name == "" {
		return ACPStdioExecutorRequest{}, errors.New("profile name is required")
	}
	if binding.Name == "" {
		return ACPStdioExecutorRequest{}, errors.New("binding name is required")
	}
	if binding.ProfileName != profile.Name {
		return ACPStdioExecutorRequest{}, fmt.Errorf(
			"binding %s references profile %s but launch requested profile %s",
			binding.Name,
			binding.ProfileName,
			profile.Name,
		)
	}

	return MapACPStdioExecutorRequest(profile, request.Prompt)
}
