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
}
