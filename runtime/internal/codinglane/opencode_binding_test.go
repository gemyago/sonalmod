package codinglane

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBindingDomainValidation(t *testing.T) {
	makeCreateParams := func() CreateOpenCodeBindingParams {
		return CreateOpenCodeBindingParams{
			Name:        "coding-default",
			ProfileName: "agent-main",
			CWD:         "/workspace/project",
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"run"},
			},
			LaunchOptions: OpenCodeLaunchOptions{
				Transport: "stdio",
			},
		}
	}

	t.Run("normalizeCreateOpenCodeBindingParams validates profile reference and binding name", func(t *testing.T) {
		t.Run("rejects invalid binding name", func(t *testing.T) {
			params := makeCreateParams()
			params.Name = "Invalid Name"

			_, err := normalizeCreateOpenCodeBindingParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid binding name")
		})

		t.Run("rejects invalid profile reference", func(t *testing.T) {
			params := makeCreateParams()
			params.ProfileName = " "

			_, err := normalizeCreateOpenCodeBindingParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "profile_name is required")
		})

		t.Run("rejects malformed profile reference", func(t *testing.T) {
			params := makeCreateParams()
			params.ProfileName = "Profile-Upper"

			_, err := normalizeCreateOpenCodeBindingParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid profile name")
		})
	})

	t.Run("applyOpenCodeBindingUpdate preserves immutable fields", func(t *testing.T) {
		existing := OpenCodeBinding{
			Name:        "binding-main",
			ProfileName: "profile-a",
			CWD:         "/workspace/first",
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"run"},
			},
			LaunchOptions: OpenCodeLaunchOptions{
				Transport: "stdio",
			},
			CreatedAt: time.Unix(100, 0).UTC(),
			UpdatedAt: time.Unix(200, 0).UTC(),
		}

		updated, err := applyOpenCodeBindingUpdate(existing, UpdateOpenCodeBindingParams{
			CWD: "/workspace/second",
			AgentCommand: OpenCodeAgentCommand{
				Command: "opencode",
				Args:    []string{"run", "--json"},
			},
			LaunchOptions: OpenCodeLaunchOptions{
				Transport: "stdio",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, existing.Name, updated.Name)
		assert.Equal(t, existing.ProfileName, updated.ProfileName)
		assert.Equal(t, existing.CreatedAt, updated.CreatedAt)
		assert.Equal(t, "/workspace/second", updated.CWD)
		assert.Equal(t, []string{"run", "--json"}, updated.AgentCommand.Args)
	})

	t.Run("OpenCodeBinding schema excludes general profile fields", func(t *testing.T) {
		typ := reflect.TypeFor[OpenCodeBinding]()
		disallowed := map[string]struct{}{
			"Role":              {},
			"Instructions":      {},
			"ToolRefs":          {},
			"ExecutionSettings": {},
		}

		for field := range typ.Fields() {
			_, found := disallowed[field.Name]
			assert.False(t, found, "field %s must stay out of OpenCode binding schema", field.Name)
		}
	})

	t.Run("normalizeAgentCommand validates command and args", func(t *testing.T) {
		_, err := normalizeAgentCommand(OpenCodeAgentCommand{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent_command.command is required")

		_, err = normalizeAgentCommand(OpenCodeAgentCommand{
			Command: "open\ncode",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control characters")

		_, err = normalizeAgentCommand(OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"run", " "},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain empty values")

		_, err = normalizeAgentCommand(OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"run", "run"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be unique")

		_, err = normalizeAgentCommand(OpenCodeAgentCommand{
			Command: "opencode",
			Args:    []string{"run", "--\tjson"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "control characters")
	})

	t.Run("normalizeLaunchOptions constrains transport", func(t *testing.T) {
		opts, err := normalizeLaunchOptions(OpenCodeLaunchOptions{})
		require.NoError(t, err)
		assert.Equal(t, "stdio", opts.Transport)

		_, err = normalizeLaunchOptions(OpenCodeLaunchOptions{Transport: "http"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be stdio")
	})

	t.Run("applyOpenCodeBindingUpdate validates payload", func(t *testing.T) {
		existing := OpenCodeBinding{
			Name:        "binding-main",
			ProfileName: "profile-main",
			CreatedAt:   time.Now().UTC(),
		}
		_, err := applyOpenCodeBindingUpdate(existing, UpdateOpenCodeBindingParams{
			AgentCommand: OpenCodeAgentCommand{},
		})
		require.Error(t, err)
	})
}
