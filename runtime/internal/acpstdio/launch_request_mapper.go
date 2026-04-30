package acpstdio

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// MapExecutorRequest composes ACP stdio executor input from profile defaults.
func MapExecutorRequest(
	profile agentprofiles.AgentProfile,
	prompt string,
) (ExecutorRequest, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ExecutorRequest{}, errors.New("prompt is required")
	}

	if profile.Name == "" {
		return ExecutorRequest{}, errors.New("profile name is required")
	}
	if profile.ExecutionSettings.ModeOrDefault() != agentprofiles.ExecutionModeACPStdio {
		return ExecutorRequest{}, fmt.Errorf(
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

	return ExecutorRequest{
		ExecutionSettings: profile.ExecutionSettings,
		Prompt:            composedPrompt,
		MCPServers:        []any{},
	}, nil
}
