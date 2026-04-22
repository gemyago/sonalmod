package agentapi

import (
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

func mapExecutionSettingsToAPI(settings ap.ExecutionSettings) AgentProfileExecutionSettings {
	return AgentProfileExecutionSettings{
		DefaultModel: settings.DefaultModel,
	}
}

func mapExecutionSettingsToInternal(settings AgentProfileExecutionSettings) ap.ExecutionSettings {
	return ap.ExecutionSettings{
		DefaultModel: settings.DefaultModel,
	}
}

func mapAgentProfileToResponse(profile ap.AgentProfile) AgentProfileResponse {
	return AgentProfileResponse{
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

func mapAgentProfilesToResponse(profiles []ap.AgentProfile) AgentProfileListResponse {
	resp := make([]AgentProfileResponse, len(profiles))
	for i, profile := range profiles {
		resp[i] = mapAgentProfileToResponse(profile)
	}
	return AgentProfileListResponse{Profiles: resp}
}
