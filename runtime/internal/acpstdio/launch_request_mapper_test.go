package acpstdio

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapExecutorRequest(t *testing.T) {
	profile := agentprofiles.AgentProfile{
		Name:         "profile-main",
		DisplayName:  "Main",
		Role:         "coding",
		Instructions: "Always include tests",
		ToolRefs:     []string{"workspacefs", "skills"},
		ExecutionSettings: agentprofiles.ExecutionSettings{
			Mode: agentprofiles.ExecutionModeACPStdio,
			AgentCommand: agentprofiles.ACPStdioAgentCommand{
				Command: "profile-opencode",
				Args:    []string{"acp", "--profile"},
			},
			Cwd: "/workspace/profile",
		},
	}

	t.Run("maps prompt with profile-owned ACP stdio settings", func(t *testing.T) {
		request, err := MapExecutorRequest(profile, "fix flaky test")
		require.NoError(t, err)
		assert.Equal(t, profile.ExecutionSettings, request.ExecutionSettings)
		assert.Contains(t, request.Prompt, profile.Instructions)
		assert.Contains(t, request.Prompt, "workspacefs")
		assert.Contains(t, request.Prompt, "fix flaky test")
		assert.Equal(t, []any{}, request.MCPServers)
	})

	t.Run("returns validation error for missing prompt", func(t *testing.T) {
		_, err := MapExecutorRequest(profile, " ")
		require.Error(t, err)
		assert.ErrorContains(t, err, "prompt is required")
	})

	t.Run("returns validation error for missing profile name", func(t *testing.T) {
		invalidProfile := profile
		invalidProfile.Name = ""
		_, err := MapExecutorRequest(invalidProfile, "run")
		require.Error(t, err)
		require.ErrorContains(t, err, "profile name is required")
	})

	t.Run("returns validation error for non ACP stdio profiles", func(t *testing.T) {
		regularProfile := profile
		regularProfile.ExecutionSettings = agentprofiles.ExecutionSettings{
			DefaultModel: "openai/gpt-5",
		}

		_, err := MapExecutorRequest(regularProfile, "run")
		require.Error(t, err)
		require.ErrorContains(t, err, "does not use acp-stdio")
	})
}
