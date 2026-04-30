package agentapi

import (
	"time"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

type agentProfileACPStdioAgentCommandPayload struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type agentProfileExecutionSettingsPayload struct {
	Mode         string                                   `json:"mode,omitempty"`
	DefaultModel string                                   `json:"defaultModel,omitempty"`
	AgentCommand *agentProfileACPStdioAgentCommandPayload `json:"agentCommand,omitempty"`
	Cwd          string                                   `json:"cwd,omitempty"`
}

type createAgentProfileRequestPayload struct {
	Name              string                               `json:"name"`
	DisplayName       *string                              `json:"displayName,omitempty"`
	Role              string                               `json:"role"`
	Instructions      string                               `json:"instructions"`
	ToolRefs          *[]string                            `json:"toolRefs,omitempty"`
	ExecutionSettings agentProfileExecutionSettingsPayload `json:"executionSettings"`
}

type updateAgentProfileRequestPayload struct {
	DisplayName       *string                              `json:"displayName,omitempty"`
	Role              string                               `json:"role"`
	Instructions      string                               `json:"instructions"`
	ToolRefs          *[]string                            `json:"toolRefs,omitempty"`
	ExecutionSettings agentProfileExecutionSettingsPayload `json:"executionSettings"`
}

type agentProfileResponsePayload struct {
	Name              string                               `json:"name"`
	DisplayName       string                               `json:"displayName"`
	Role              string                               `json:"role"`
	Instructions      string                               `json:"instructions"`
	ToolRefs          []string                             `json:"toolRefs"`
	ExecutionSettings agentProfileExecutionSettingsPayload `json:"executionSettings"`
	CreatedAt         time.Time                            `json:"createdAt"`
	UpdatedAt         time.Time                            `json:"updatedAt"`
}

type agentProfileListResponsePayload struct {
	Profiles []agentProfileResponsePayload `json:"profiles"`
}

func mapExecutionSettingsToAPI(settings ap.ExecutionSettings) agentProfileExecutionSettingsPayload {
	payload := agentProfileExecutionSettingsPayload{
		Mode:         string(settings.Mode),
		DefaultModel: settings.DefaultModel,
		Cwd:          settings.Cwd,
	}

	if settings.ModeOrDefault() == ap.ExecutionModeACPStdio ||
		settings.AgentCommand.Command != "" ||
		len(settings.AgentCommand.Args) > 0 ||
		settings.Cwd != "" {
		payload.AgentCommand = &agentProfileACPStdioAgentCommandPayload{
			Command: settings.AgentCommand.Command,
			Args:    append([]string(nil), settings.AgentCommand.Args...),
		}
	}

	return payload
}

func mapExecutionSettingsToInternal(settings agentProfileExecutionSettingsPayload) ap.ExecutionSettings {
	mapped := ap.ExecutionSettings{
		Mode:         ap.ExecutionMode(settings.Mode),
		DefaultModel: settings.DefaultModel,
		Cwd:          settings.Cwd,
	}
	if settings.AgentCommand != nil {
		mapped.AgentCommand = ap.ACPStdioAgentCommand{
			Command: settings.AgentCommand.Command,
			Args:    append([]string(nil), settings.AgentCommand.Args...),
		}
	}

	return mapped
}

func mapAgentProfileToResponse(profile ap.AgentProfile) agentProfileResponsePayload {
	return agentProfileResponsePayload{
		Name:              profile.Name,
		DisplayName:       profile.DisplayName,
		Role:              profile.Role,
		Instructions:      profile.Instructions,
		ToolRefs:          append([]string(nil), profile.ToolRefs...),
		ExecutionSettings: mapExecutionSettingsToAPI(profile.ExecutionSettings),
		CreatedAt:         profile.CreatedAt,
		UpdatedAt:         profile.UpdatedAt,
	}
}

func mapAgentProfilesToResponse(profiles []ap.AgentProfile) agentProfileListResponsePayload {
	resp := make([]agentProfileResponsePayload, len(profiles))
	for i, profile := range profiles {
		resp[i] = mapAgentProfileToResponse(profile)
	}

	return agentProfileListResponsePayload{Profiles: resp}
}
