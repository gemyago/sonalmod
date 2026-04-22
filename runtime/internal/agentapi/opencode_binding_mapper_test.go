//go:build !release

package agentapi

import (
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/stretchr/testify/assert"
)

func TestOpenCodeBindingMapper(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	binding := agent.OpenCodeBinding{
		Name:        "binding-main",
		ProfileName: "profile-main",
		CWD:         "/tmp/main",
		AgentCommand: agent.OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"--fast"},
		},
		LaunchOptions: agent.OpenCodeLaunchOptions{Transport: "stdio"},
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	t.Run("mapOpenCodeBindingToResponse", func(t *testing.T) {
		t.Parallel()
		resp := mapOpenCodeBindingToResponse(binding)
		assert.Equal(t, binding.Name, resp.Name)
		assert.Equal(t, binding.ProfileName, resp.ProfileName)
		assert.Equal(t, binding.CWD, resp.Cwd)
		assert.Equal(t, binding.AgentCommand.Command, resp.AgentCommand.Command)
		assert.Equal(t, binding.AgentCommand.Args, resp.AgentCommand.Args)
		assert.Equal(t, binding.LaunchOptions.Transport, resp.LaunchOptions.Transport)
		assert.Equal(t, binding.CreatedAt, resp.CreatedAt)
		assert.Equal(t, binding.UpdatedAt, resp.UpdatedAt)
	})

	t.Run("mapOpenCodeBindingsToResponse", func(t *testing.T) {
		t.Parallel()
		resp := mapOpenCodeBindingsToResponse([]agent.OpenCodeBinding{binding})
		assert.Len(t, resp.Bindings, 1)
		assert.Equal(t, binding.Name, resp.Bindings[0].Name)
	})
}
