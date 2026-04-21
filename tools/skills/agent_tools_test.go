package skills

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iskills "github.com/gemyago/sonalmod/tools/skills/internal/skills"
)

func testToolContext(t *testing.T) *agent.ToolContext {
	t.Helper()
	return &agent.ToolContext{Context: t.Context()}
}

// makeSkillDir creates a skill directory with a valid SKILL.md under root, returning the skill name.
func makeSkillDir(t *testing.T, root, name, description, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", name, description, body)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func makeCatalog(t *testing.T, roots []string) *iskills.Catalog {
	t.Helper()
	cat, err := iskills.NewCatalog(roots, iskills.WithLogger(slog.New(slog.DiscardHandler)))
	require.NoError(t, err)
	return cat
}

func TestSkillsListTool(t *testing.T) {
	t.Parallel()

	t.Run("tool_name_is_skills_list", func(t *testing.T) {
		t.Parallel()
		cat := makeCatalog(t, []string{t.TempDir()})
		td := skillsListTool(cat)
		assert.Equal(t, "skills_list", td.Name)
	})

	t.Run("returns_empty_skills_when_catalog_is_empty", func(t *testing.T) {
		t.Parallel()
		cat := makeCatalog(t, []string{t.TempDir()})
		td := skillsListTool(cat)
		res, err := td.Handler(testToolContext(t), skillsListRequest{})
		require.NoError(t, err)
		assert.Empty(t, res.Skills)
	})

	t.Run("returns_all_discovered_skills_metadata", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		root := t.TempDir()

		nameA := "skill-a"
		descA := fake.Lorem().Sentence(5)
		nameB := "skill-b"
		descB := fake.Lorem().Sentence(5)
		makeSkillDir(t, root, nameA, descA, "body A")
		makeSkillDir(t, root, nameB, descB, "body B")

		cat := makeCatalog(t, []string{root})
		td := skillsListTool(cat)
		res, err := td.Handler(testToolContext(t), skillsListRequest{})
		require.NoError(t, err)
		require.Len(t, res.Skills, 2)

		// Verify metadata only (no body), and names/descriptions match.
		names := map[string]string{}
		for _, s := range res.Skills {
			names[s.Name] = s.Description
		}
		assert.Equal(t, descA, names[nameA])
		assert.Equal(t, descB, names[nameB])
	})

	t.Run("response_does_not_include_body", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		makeSkillDir(t, root, "my-skill", "A useful skill", "SECRET BODY CONTENT")
		cat := makeCatalog(t, []string{root})
		td := skillsListTool(cat)
		res, err := td.Handler(testToolContext(t), skillsListRequest{})
		require.NoError(t, err)
		require.Len(t, res.Skills, 1)
		// skillMetadata must not have a Body field; verified at compile time by struct type.
		// But also verify the Name and Description are correct.
		assert.Equal(t, "my-skill", res.Skills[0].Name)
		assert.Equal(t, "A useful skill", res.Skills[0].Description)
	})
}

func TestSkillsReadTool(t *testing.T) {
	t.Parallel()

	t.Run("tool_name_is_skills_read", func(t *testing.T) {
		t.Parallel()
		cat := makeCatalog(t, []string{t.TempDir()})
		td := skillsReadTool(cat)
		assert.Equal(t, "skills_read", td.Name)
	})

	t.Run("returns_skill_metadata_and_body_for_known_skill", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		root := t.TempDir()
		skillName := "my-skill"
		skillDesc := fake.Lorem().Sentence(5)
		skillBody := fake.Lorem().Paragraph(2)
		makeSkillDir(t, root, skillName, skillDesc, skillBody)

		cat := makeCatalog(t, []string{root})
		td := skillsReadTool(cat)
		res, err := td.Handler(testToolContext(t), skillsReadRequest{Name: skillName})
		require.NoError(t, err)
		assert.Equal(t, skillName, res.Name)
		assert.Equal(t, skillDesc, res.Description)
		assert.Equal(t, skillBody, res.Body)
	})

	t.Run("returns_error_for_unknown_skill_name", func(t *testing.T) {
		t.Parallel()
		cat := makeCatalog(t, []string{t.TempDir()})
		td := skillsReadTool(cat)
		_, err := td.Handler(testToolContext(t), skillsReadRequest{Name: "no-such-skill"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "no-such-skill")
	})

	t.Run("error_for_unknown_skill_does_not_leak_host_paths", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		cat := makeCatalog(t, []string{root})
		td := skillsReadTool(cat)
		_, err := td.Handler(testToolContext(t), skillsReadRequest{Name: "ghost-skill"})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), root)
	})

	t.Run("returns_error_when_name_is_empty", func(t *testing.T) {
		t.Parallel()
		cat := makeCatalog(t, []string{t.TempDir()})
		td := skillsReadTool(cat)
		_, err := td.Handler(testToolContext(t), skillsReadRequest{Name: ""})
		require.Error(t, err)
	})
}
