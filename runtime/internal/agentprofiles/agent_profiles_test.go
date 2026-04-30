package agentprofiles

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentProfilesDomainValidation(t *testing.T) {
	makeCreateParams := func() CreateAgentProfileParams {
		return CreateAgentProfileParams{
			Name:         "profile-one",
			DisplayName:  " Profile One ",
			Role:         " assistant ",
			Instructions: " do things ",
			ToolRefs:     []string{" tool.read ", "tool.write", "tool.read"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: " provider/model ",
			},
		}
	}

	t.Run("normalizeCreateParams validates identifier", func(t *testing.T) {
		t.Run("rejects uppercase identifier", func(t *testing.T) {
			params := makeCreateParams()
			params.Name = "Profile-Upper"

			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid profile name")
		})

		t.Run("rejects identifier starting with digit", func(t *testing.T) {
			params := makeCreateParams()
			params.Name = "1profile"

			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid profile name")
		})

		t.Run("accepts valid identifier and trims whitespace", func(t *testing.T) {
			params := makeCreateParams()
			params.Name = " profile-1 "

			normalized, err := normalizeCreateParams(params)
			require.NoError(t, err)
			assert.Equal(t, "profile-1", normalized.Name)
		})
	})

	t.Run("normalizeCreateParams normalizes tool refs", func(t *testing.T) {
		t.Run("deduplicates while preserving first-seen order", func(t *testing.T) {
			params := makeCreateParams()

			normalized, err := normalizeCreateParams(params)
			require.NoError(t, err)
			assert.Equal(t, []string{"tool.read", "tool.write"}, normalized.ToolRefs)
		})

		t.Run("rejects empty tool refs", func(t *testing.T) {
			params := makeCreateParams()
			params.ToolRefs = []string{"tool.read", "  "}

			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tool_refs")
		})
	})

	t.Run("normalizeExecutionSettings supports regular and acp-stdio modes", func(t *testing.T) {
		t.Run("treats omitted mode as regular", func(t *testing.T) {
			normalized, err := normalizeExecutionSettings(ExecutionSettings{
				DefaultModel: " openai/gpt-4.1 ",
			})
			require.NoError(t, err)
			assert.Empty(t, normalized.Mode)
			assert.Equal(t, ExecutionModeRegular, normalized.ModeOrDefault())
			assert.Equal(t, "openai/gpt-4.1", normalized.DefaultModel)
		})

		t.Run("accepts explicit regular mode", func(t *testing.T) {
			normalized, err := normalizeExecutionSettings(ExecutionSettings{
				Mode:         ExecutionModeRegular,
				DefaultModel: " openai/gpt-4.1 ",
			})
			require.NoError(t, err)
			assert.Equal(t, ExecutionModeRegular, normalized.Mode)
			assert.Equal(t, "openai/gpt-4.1", normalized.DefaultModel)
		})

		t.Run("accepts acp-stdio mode", func(t *testing.T) {
			normalized, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeACPStdio,
				AgentCommand: ACPStdioAgentCommand{
					Command: " opencode ",
					Args:    []string{" acp ", "--safe"},
				},
				Cwd: " /workspace ",
			})
			require.NoError(t, err)
			assert.Equal(t, ExecutionModeACPStdio, normalized.Mode)
			assert.Equal(t, ACPStdioAgentCommand{
				Command: "opencode",
				Args:    []string{"acp", "--safe"},
			}, normalized.AgentCommand)
			assert.Equal(t, "/workspace", normalized.Cwd)
			assert.Empty(t, normalized.DefaultModel)
		})
	})

	t.Run("normalizeExecutionSettings rejects invalid execution settings", func(t *testing.T) {
		t.Run("rejects unsupported mode", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode:         "remote",
				DefaultModel: "openai/gpt-4.1",
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.mode")
		})

		t.Run("rejects regular mode without default model", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeRegular,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.default_model")
		})

		t.Run("rejects regular mode with acp settings", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				DefaultModel: "openai/gpt-4.1",
				AgentCommand: ACPStdioAgentCommand{
					Command: "opencode",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.agent_command")
		})

		t.Run("rejects acp-stdio mode with regular default model", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode:         ExecutionModeACPStdio,
				DefaultModel: "openai/gpt-4.1",
				AgentCommand: ACPStdioAgentCommand{Command: "opencode"},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.default_model")
		})

		t.Run("rejects acp-stdio mode without command", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeACPStdio,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.agent_command.command")
		})

		t.Run("rejects acp-stdio mode with empty arg", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeACPStdio,
				AgentCommand: ACPStdioAgentCommand{
					Command: "opencode",
					Args:    []string{"acp", " "},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.agent_command.args")
		})

		t.Run("rejects acp-stdio mode with duplicate args", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeACPStdio,
				AgentCommand: ACPStdioAgentCommand{
					Command: "opencode",
					Args:    []string{"acp", " acp "},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must be unique")
		})

		t.Run("rejects acp-stdio mode with control chars", func(t *testing.T) {
			_, err := normalizeExecutionSettings(ExecutionSettings{
				Mode: ExecutionModeACPStdio,
				AgentCommand: ACPStdioAgentCommand{
					Command: "open\ncode",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "control characters")
		})
	})

	t.Run("applyProfileUpdate preserves immutable fields", func(t *testing.T) {
		createdAt := time.Now().Add(-2 * time.Hour).UTC()
		existing := AgentProfile{
			Name:         "profile-main",
			DisplayName:  "Old Name",
			Role:         "assistant",
			Instructions: "original instructions",
			ToolRefs:     []string{"tool.read"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model-a",
			},
			CreatedAt: createdAt,
			UpdatedAt: time.Now().Add(-1 * time.Hour).UTC(),
		}

		updated, err := applyProfileUpdate(existing, UpdateAgentProfileParams{
			DisplayName:  " New Name ",
			Role:         " planner ",
			Instructions: " new instructions ",
			ToolRefs:     []string{" tool.write ", "tool.read", "tool.write"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: " provider/model-b ",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, existing.Name, updated.Name)
		assert.Equal(t, existing.CreatedAt, updated.CreatedAt)
		assert.Equal(t, "New Name", updated.DisplayName)
		assert.Equal(t, "planner", updated.Role)
		assert.Equal(t, "new instructions", updated.Instructions)
		assert.Equal(t, []string{"tool.write", "tool.read"}, updated.ToolRefs)
		assert.Equal(t, "provider/model-b", updated.ExecutionSettings.DefaultModel)
	})

	t.Run("normalizeCreateParams validates required fields", func(t *testing.T) {
		t.Run("rejects empty role", func(t *testing.T) {
			params := makeCreateParams()
			params.Role = "  "
			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "role is required")
		})

		t.Run("rejects empty instructions", func(t *testing.T) {
			params := makeCreateParams()
			params.Instructions = "  "
			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "instructions are required")
		})

		t.Run("rejects empty default model", func(t *testing.T) {
			params := makeCreateParams()
			params.ExecutionSettings.DefaultModel = " "
			_, err := normalizeCreateParams(params)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.default_model")
		})
	})

	t.Run("applyProfileUpdate validates required fields", func(t *testing.T) {
		existing := AgentProfile{
			Name:         "profile-main",
			DisplayName:  "Old Name",
			Role:         "assistant",
			Instructions: "original instructions",
			ToolRefs:     []string{"tool.read"},
			ExecutionSettings: ExecutionSettings{
				DefaultModel: "provider/model-a",
			},
		}

		t.Run("rejects empty role", func(t *testing.T) {
			_, err := applyProfileUpdate(existing, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         " ",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "role is required")
		})

		t.Run("rejects empty instructions", func(t *testing.T) {
			_, err := applyProfileUpdate(existing, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: " ",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "instructions are required")
		})

		t.Run("rejects empty default model", func(t *testing.T) {
			_, err := applyProfileUpdate(existing, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ExecutionSettings: ExecutionSettings{
					DefaultModel: " ",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "execution_settings.default_model")
		})

		t.Run("rejects empty tool refs", func(t *testing.T) {
			_, err := applyProfileUpdate(existing, UpdateAgentProfileParams{
				DisplayName:  "x",
				Role:         "assistant",
				Instructions: "ok",
				ToolRefs:     []string{"tool.read", " "},
				ExecutionSettings: ExecutionSettings{
					DefaultModel: "provider/model",
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "tool_refs")
		})
	})
}
