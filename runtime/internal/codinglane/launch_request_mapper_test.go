package codinglane

import (
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeLaunchRequestMapper(t *testing.T) {
	profile := agentprofiles.AgentProfile{
		Name:         "profile-main",
		DisplayName:  "Main",
		Role:         "coding",
		Instructions: "Always include tests",
		ToolRefs:     []string{"workspacefs", "skills"},
		ExecutionSettings: agentprofiles.ExecutionSettings{
			DefaultModel: "openai/gpt-5",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	binding := OpenCodeBinding{
		Name:        "binding-main",
		ProfileName: profile.Name,
		CWD:         "/workspace/project",
		AgentCommand: OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"acp"},
		},
		LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
	}

	t.Run("merges profile and binding defaults into ACP request", func(t *testing.T) {
		request, err := MapOpenCodeLaunchRequest(profile, binding, OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      "fix flaky test",
		})
		require.NoError(t, err)
		assert.Equal(t, binding.AgentCommand, request.AgentCommand)
		assert.Equal(t, binding.CWD, request.CWD)
		assert.Contains(t, request.Prompt, profile.Instructions)
		assert.Contains(t, request.Prompt, "openai/gpt-5")
		assert.Contains(t, request.Prompt, "workspacefs")
		assert.Contains(t, request.Prompt, "fix flaky test")
	})

	t.Run("returns validation error for missing prompt", func(t *testing.T) {
		_, err := MapOpenCodeLaunchRequest(profile, binding, OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      " ",
		})
		require.Error(t, err)
		assert.ErrorContains(t, err, "prompt is required")
	})
}
