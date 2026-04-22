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

// MapOpenCodeLaunchRequest composes ACP launch params from profile and binding defaults.
func MapOpenCodeLaunchRequest(
	profile agentprofiles.AgentProfile,
	binding OpenCodeBinding,
	request OpenCodeLaunchRequest,
) (OpenCodeACPLaunchRequest, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return OpenCodeACPLaunchRequest{}, errors.New("prompt is required")
	}

	if profile.Name == "" {
		return OpenCodeACPLaunchRequest{}, errors.New("profile name is required")
	}
	if binding.Name == "" {
		return OpenCodeACPLaunchRequest{}, errors.New("binding name is required")
	}
	if binding.ProfileName != profile.Name {
		return OpenCodeACPLaunchRequest{}, fmt.Errorf(
			"binding %s references profile %s but launch requested profile %s",
			binding.Name,
			binding.ProfileName,
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

	return OpenCodeACPLaunchRequest{
		AgentCommand: binding.AgentCommand,
		CWD:          binding.CWD,
		Prompt:       composedPrompt,
		MCPServers:   []any{},
	}, nil
}
