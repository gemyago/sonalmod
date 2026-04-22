package agentapi

import "github.com/gemyago/sonalmod/runtime/agent"

func mapOpenCodeBindingToResponse(binding agent.OpenCodeBinding) OpenCodeBindingResponse {
	return OpenCodeBindingResponse{
		Name:        binding.Name,
		ProfileName: binding.ProfileName,
		Cwd:         binding.CWD,
		AgentCommand: OpenCodeAgentCommand{
			Command: binding.AgentCommand.Command,
			Args:    append([]string(nil), binding.AgentCommand.Args...),
		},
		LaunchOptions: OpenCodeLaunchOptions{
			Transport: binding.LaunchOptions.Transport,
		},
		CreatedAt: binding.CreatedAt,
		UpdatedAt: binding.UpdatedAt,
	}
}

func mapOpenCodeBindingsToResponse(bindings []agent.OpenCodeBinding) OpenCodeBindingListResponse {
	resp := make([]OpenCodeBindingResponse, len(bindings))
	for idx, binding := range bindings {
		resp[idx] = mapOpenCodeBindingToResponse(binding)
	}
	return OpenCodeBindingListResponse{Bindings: resp}
}
