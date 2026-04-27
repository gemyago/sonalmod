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
			Mode: agentprofiles.ExecutionModeACPStdio,
			AgentCommand: agentprofiles.ACPStdioAgentCommand{
				Command: "profile-opencode",
				Args:    []string{"acp", "--profile"},
			},
			Cwd: "/workspace/profile",
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	binding := OpenCodeBinding{
		Name:        "binding-main",
		ProfileName: profile.Name,
		CWD:         "/workspace/binding",
		AgentCommand: OpenCodeAgentCommand{
			Command: "binding-opencode",
			Args:    []string{"acp", "--binding"},
		},
		LaunchOptions: OpenCodeLaunchOptions{Transport: "stdio"},
	}

	t.Run("maps prompt with profile-owned ACP stdio settings", func(t *testing.T) {
		request, err := MapOpenCodeLaunchRequest(profile, binding, OpenCodeLaunchRequest{
			ProfileName: profile.Name,
			Prompt:      "fix flaky test",
		})
		require.NoError(t, err)
		assert.Equal(t, profile.ExecutionSettings, request.ExecutionSettings)
		assert.Contains(t, request.Prompt, profile.Instructions)
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

	t.Run("returns validation error for missing profile and binding names", func(t *testing.T) {
		invalidProfile := profile
		invalidProfile.Name = ""
		_, err := MapOpenCodeLaunchRequest(invalidProfile, binding, OpenCodeLaunchRequest{
			Prompt: "run",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "profile name is required")

		invalidBinding := binding
		invalidBinding.Name = ""
		_, err = MapOpenCodeLaunchRequest(profile, invalidBinding, OpenCodeLaunchRequest{
			Prompt: "run",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "binding name is required")
	})

	t.Run("returns validation error when binding references another profile", func(t *testing.T) {
		invalidBinding := binding
		invalidBinding.ProfileName = "other-profile"
		_, err := MapOpenCodeLaunchRequest(profile, invalidBinding, OpenCodeLaunchRequest{
			Prompt: "run",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "references profile")
	})

	t.Run("returns validation error for non ACP stdio profiles", func(t *testing.T) {
		regularProfile := profile
		regularProfile.ExecutionSettings = agentprofiles.ExecutionSettings{
			DefaultModel: "openai/gpt-5",
		}

		_, err := MapOpenCodeLaunchRequest(regularProfile, binding, OpenCodeLaunchRequest{
			Prompt: "run",
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "does not use acp-stdio")
	})
}
