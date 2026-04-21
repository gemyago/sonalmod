package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSkillFile creates a skill directory + SKILL.md under root.
func writeSkillFile(t *testing.T, root, name, description string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\nbody of " + name
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}

func TestNewCatalog(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("empty_roots_returns_empty_catalog", func(t *testing.T) {
		t.Parallel()
		cat, err := NewCatalog(nil, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		assert.Empty(t, cat.List())
	})

	t.Run("omitting_WithLogger_uses_slog_Default", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeSkillFile(t, root, "default-log-skill", fake.Lorem().Sentence(4))
		cat, err := NewCatalog([]string{root})
		require.NoError(t, err)
		require.Len(t, cat.List(), 1)
		assert.Equal(t, "default-log-skill", cat.List()[0].Name)
	})

	t.Run("nonexistent_root_is_skipped_with_warning", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "no-such-dir")
		cat, err := NewCatalog([]string{missing}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		assert.Empty(t, cat.List())
	})

	t.Run("root_with_no_skill_dirs_returns_empty_catalog", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		assert.Empty(t, cat.List())
	})

	t.Run("single_valid_skill_discovered", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		name := "my-skill"
		desc := fake.Lorem().Sentence(6)
		writeSkillFile(t, root, name, desc)

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 1)
		assert.Equal(t, name, entries[0].Name)
		assert.Equal(t, desc, entries[0].Description)
	})

	t.Run("multiple_skills_in_single_root_all_discovered", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		descA := fake.Lorem().Sentence(6)
		descB := fake.Lorem().Sentence(6)
		writeSkillFile(t, root, "a-skill", descA)
		writeSkillFile(t, root, "b-skill", descB)

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 2)

		names := []string{entries[0].Name, entries[1].Name}
		assert.Contains(t, names, "a-skill")
		assert.Contains(t, names, "b-skill")
	})

	t.Run("multiple_roots_all_skills_discovered", func(t *testing.T) {
		t.Parallel()
		rootA := t.TempDir()
		rootB := t.TempDir()
		writeSkillFile(t, rootA, "skill-a", fake.Lorem().Sentence(6))
		writeSkillFile(t, rootB, "skill-b", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{rootA, rootB}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 2)
		names := []string{entries[0].Name, entries[1].Name}
		assert.Contains(t, names, "skill-a")
		assert.Contains(t, names, "skill-b")
	})

	t.Run("duplicate_skill_name_first_root_wins", func(t *testing.T) {
		t.Parallel()
		rootA := t.TempDir()
		rootB := t.TempDir()
		descFirst := fake.Lorem().Sentence(6)
		descSecond := fake.Lorem().Sentence(6)
		writeSkillFile(t, rootA, "dup-skill", descFirst)
		writeSkillFile(t, rootB, "dup-skill", descSecond)

		cat, err := NewCatalog([]string{rootA, rootB}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 1)
		assert.Equal(t, descFirst, entries[0].Description)
	})

	t.Run("invalid_skill_file_is_skipped_without_error", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Create a skill dir with invalid SKILL.md (missing required fields)
		dir := filepath.Join(root, "bad-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("no frontmatter"), 0o644))

		// Add a valid skill too
		validDesc := fake.Lorem().Sentence(6)
		writeSkillFile(t, root, "good-skill", validDesc)

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 1)
		assert.Equal(t, "good-skill", entries[0].Name)
	})

	t.Run("dir_without_SKILL_md_is_skipped", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// subdirectory but no SKILL.md
		require.NoError(t, os.MkdirAll(filepath.Join(root, "not-a-skill"), 0o755))
		writeSkillFile(t, root, "real-skill", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		assert.Len(t, cat.List(), 1)
	})

	t.Run("catalog_max_entries_limit_truncates_results", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Create 3 skills but limit to 2
		writeSkillFile(t, root, "a-skill", fake.Lorem().Sentence(6))
		writeSkillFile(t, root, "b-skill", fake.Lorem().Sentence(6))
		writeSkillFile(t, root, "c-skill", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)), WithMaxCatalogEntries(2))
		require.NoError(t, err)
		assert.Len(t, cat.List(), 2)
	})

	t.Run("skill_file_exceeding_max_bytes_is_skipped", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		name := "big-skill"
		desc := fake.Lorem().Sentence(6)
		bigBody := strings.Repeat("x", 200)
		dir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + bigBody
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		// limit to very small size so the file exceeds it
		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)), WithMaxSkillBytes(50))
		require.NoError(t, err)
		assert.Empty(t, cat.List())
	})
}

func TestCatalog(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("Get_returns_entry_for_known_skill", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		name := "known-skill"
		desc := fake.Lorem().Sentence(6)
		writeSkillFile(t, root, name, desc)

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entry, ok := cat.Get(name)
		require.True(t, ok)
		assert.Equal(t, name, entry.Name)
		assert.Equal(t, desc, entry.Description)
		assert.NotEmpty(t, entry.Body)
	})

	t.Run("Get_returns_false_for_unknown_skill", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeSkillFile(t, root, "existing-skill", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		_, ok := cat.Get("nonexistent-skill")
		assert.False(t, ok)
	})

	t.Run("List_does_not_expose_host_paths", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeSkillFile(t, root, "safe-skill", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		entries := cat.List()
		require.Len(t, entries, 1)
		// Name, Description must not contain the root path
		assert.NotContains(t, entries[0].Name, root)
		assert.NotContains(t, entries[0].Description, root)
	})
}

func TestCatalog_nil_receiver(t *testing.T) {
	t.Parallel()
	var cat *Catalog

	t.Run("List_returns_nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, cat.List())
	})

	t.Run("Get_returns_false", func(t *testing.T) {
		t.Parallel()
		_, ok := cat.Get("any-skill")
		assert.False(t, ok)
	})
}

func TestNewCatalog_max_entries_across_roots(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("catalog_limit_stops_scanning_subsequent_roots", func(t *testing.T) {
		t.Parallel()
		rootA := t.TempDir()
		rootB := t.TempDir()
		// Fill rootA to the limit
		writeSkillFile(t, rootA, "a-skill", fake.Lorem().Sentence(6))
		writeSkillFile(t, rootB, "b-skill", fake.Lorem().Sentence(6))

		// Limit to 1: rootA fills the catalog, rootB root should not be scanned
		cat, err := NewCatalog(
			[]string{rootA, rootB},
			WithLogger(slog.New(slog.DiscardHandler)),
			WithMaxCatalogEntries(1),
		)
		require.NoError(t, err)
		assert.Len(t, cat.List(), 1)
		assert.Equal(t, "a-skill", cat.List()[0].Name)
	})
}

func TestNewCatalog_non_dir_entries_ignored(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("regular_files_in_root_are_skipped", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		// Create a regular file at the root level (not a dir)
		require.NoError(t, os.WriteFile(filepath.Join(root, "not-a-dir.md"), []byte("content"), 0o644))
		// Add valid skill
		writeSkillFile(t, root, "real-skill", fake.Lorem().Sentence(6))

		cat, err := NewCatalog([]string{root}, WithLogger(slog.New(slog.DiscardHandler)))
		require.NoError(t, err)
		require.Len(t, cat.List(), 1)
		assert.Equal(t, "real-skill", cat.List()[0].Name)
	})
}
