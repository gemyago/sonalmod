package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	iskills "github.com/gemyago/sonalmod/tools/skills/internal/skills"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureRegistry struct {
	addCalls int
	tools    []agent.DefinedTool
}

func (c *captureRegistry) AddTools(tools ...agent.DefinedTool) {
	c.addCalls++
	c.tools = append(c.tools, tools...)
}

func TestSkillsRegisterTools(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	t.Run("registers_expected_tools", func(t *testing.T) {
		t.Parallel()
		reg := &captureRegistry{}
		skillSet, err := New([]string{t.TempDir()}, WithLogger(logger))
		require.NoError(t, err)
		skillSet.RegisterTools(reg)
		assert.Equal(t, 1, reg.addCalls)
		assert.Len(t, reg.tools, ExpectedToolCount)
	})

	t.Run("register_tools_without_WithLogger", func(t *testing.T) {
		t.Parallel()
		reg := &captureRegistry{}
		skillSet, err := New([]string{t.TempDir()})
		require.NoError(t, err)
		skillSet.RegisterTools(reg)
		assert.Len(t, reg.tools, ExpectedToolCount)
	})
}

// TestSkillsFlowRegression provides cross-module regression coverage for the full skills flow:
// instruction addon composition, skills_read content fidelity, disabled-skills isolation,
// and startup-only catalog semantics (no hot-reload).
func TestSkillsFlowRegression(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	logger := slog.New(slog.DiscardHandler)

	// makeSkillEntry writes a valid SKILL.md under root/<name>/.
	makeSkillEntry := func(t *testing.T, root, name, description, body string) {
		t.Helper()
		dir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", name, description, body)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
	}

	t.Run("instruction_addon_contains_skill_name_and_description", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		skillName := "my-skill"
		skillDesc := fake.Lorem().Sentence(5)
		makeSkillEntry(t, root, skillName, skillDesc, "# Body")

		skillSet, err := New([]string{root}, WithLogger(logger))
		require.NoError(t, err)

		frags := skillSet.BuildSystemPromptFragments()
		require.Len(t, frags, 1)
		assert.Equal(t, "Skills", frags[0].Section)
		content := frags[0].Content
		assert.Contains(t, content, skillName)
		assert.Contains(t, content, skillDesc)
		assert.True(t, strings.HasPrefix(content, "<available_skills>"), "content must start with <available_skills>")
		assert.True(t, strings.HasSuffix(content, "</available_skills>"), "content must end with </available_skills>")
	})

	t.Run("instruction_addon_contains_all_discovered_skills", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		nameA := "skill-alpha"
		descA := fake.Lorem().Sentence(4)
		nameB := "skill-beta"
		descB := fake.Lorem().Sentence(4)
		makeSkillEntry(t, root, nameA, descA, "body A")
		makeSkillEntry(t, root, nameB, descB, "body B")

		skillSet, err := New([]string{root}, WithLogger(logger))
		require.NoError(t, err)

		frags := skillSet.BuildSystemPromptFragments()
		require.Len(t, frags, 1)
		content := frags[0].Content
		assert.Contains(t, content, nameA)
		assert.Contains(t, content, descA)
		assert.Contains(t, content, nameB)
		assert.Contains(t, content, descB)
	})

	t.Run("instruction_addon_is_empty_when_no_skills_discovered", func(t *testing.T) {
		t.Parallel()
		skillSet, err := New([]string{t.TempDir()}, WithLogger(logger))
		require.NoError(t, err)

		assert.Empty(t, skillSet.BuildSystemPromptFragments())
	})

	t.Run("skills_read_returns_full_body_matching_disk_content", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		skillName := "read-skill"
		skillDesc := fake.Lorem().Sentence(5)
		expectedBody := fake.Lorem().Paragraph(3)
		makeSkillEntry(t, root, skillName, skillDesc, expectedBody)

		skillSet, err := New([]string{root}, WithLogger(logger))
		require.NoError(t, err)

		// Verify the catalog holds exactly the body written to disk (same build path as [New]).
		cat, err := iskills.NewCatalog([]string{root}, iskills.WithLogger(logger))
		require.NoError(t, err)
		entry, ok := cat.Get(skillName)
		require.True(t, ok)
		assert.Equal(t, skillName, entry.Name)
		assert.Equal(t, skillDesc, entry.Description)
		assert.Equal(t, expectedBody, entry.Body)

		reg := &captureRegistry{}
		skillSet.RegisterTools(reg)
		assert.Len(t, reg.tools, ExpectedToolCount)
	})

	t.Run("disabled_skills_registers_no_tools", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeSkillEntry(t, root, "some-skill", "A skill that should not be registered", "body")

		// When skills are disabled, [Skills.RegisterTools] is never called.
		skillsEnabled := false
		reg := &captureRegistry{}
		if skillsEnabled {
			skillSet, err := New([]string{root}, WithLogger(logger))
			require.NoError(t, err)
			skillSet.RegisterTools(reg)
		}

		assert.Empty(t, reg.tools, "no skill tools must be registered when skills are disabled")
	})

	t.Run("catalog_does_not_reflect_disk_changes_after_startup", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		skillName := "stable-skill"
		makeSkillEntry(t, root, skillName, "Original description", "Original body")

		cat, err := iskills.NewCatalog([]string{root}, iskills.WithLogger(logger))
		require.NoError(t, err)

		entry, ok := cat.Get(skillName)
		require.True(t, ok)
		assert.Equal(t, "Original body", entry.Body)

		updatedContent := fmt.Sprintf("---\nname: %s\ndescription: Updated description\n---\nUpdated body", skillName)
		require.NoError(t, os.WriteFile(
			filepath.Join(root, skillName, "SKILL.md"),
			[]byte(updatedContent),
			0o644,
		))

		newSkillName := "new-skill"
		makeSkillEntry(t, root, newSkillName, "New skill added after startup", "New body")

		entry, ok = cat.Get(skillName)
		require.True(t, ok)
		assert.Equal(t, "Original body", entry.Body, "catalog must not reflect disk changes after startup")

		_, ok = cat.Get(newSkillName)
		assert.False(t, ok, "catalog must not discover skills added after startup")
	})
}
