package skills

import (
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkillFile(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run("valid_frontmatter_and_body_extracted", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		desc := fake.Lorem().Sentence(8)
		body := fake.Lorem().Paragraph(3)
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n" + body

		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Equal(t, name, got.Name)
		assert.Equal(t, desc, got.Description)
		assert.Equal(t, body, got.Body)
	})

	t.Run("optional_fields_parsed_when_present", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		desc := fake.Lorem().Sentence(6)
		license := "MIT"
		compat := "cursor>=1.0"
		allowed := "read_file write_file"
		body := fake.Lorem().Paragraph(2)
		content := "---\nname: " + name + "\ndescription: " + desc +
			"\nlicense: " + license +
			"\ncompatibility: " + compat +
			"\nallowed-tools: " + allowed +
			"\n---\n" + body

		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Equal(t, license, got.License)
		assert.Equal(t, compat, got.Compatibility)
		assert.Equal(t, allowed, got.AllowedTools)
	})

	t.Run("missing_frontmatter_delimiter_returns_error", func(t *testing.T) {
		t.Parallel()
		content := "name: my-skill\ndescription: some desc\nno frontmatter here"
		_, err := parseSkillFile([]byte(content), "my-skill")
		require.Error(t, err)
	})

	t.Run("unclosed_frontmatter_delimiter_returns_error", func(t *testing.T) {
		t.Parallel()
		content := "---\nname: my-skill\ndescription: some desc\nno closing delimiter"
		_, err := parseSkillFile([]byte(content), "my-skill")
		require.Error(t, err)
	})

	t.Run("body_without_leading_newline_after_delimiter", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		desc := fake.Lorem().Sentence(6)
		// Closing delimiter immediately followed by body (no newline)
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---body here"
		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Equal(t, "body here", got.Body)
	})

	t.Run("malformed_yaml_returns_error", func(t *testing.T) {
		t.Parallel()
		content := "---\nname: [unclosed\n---\nbody"
		_, err := parseSkillFile([]byte(content), "my-skill")
		require.Error(t, err)
	})

	t.Run("missing_name_field_returns_error", func(t *testing.T) {
		t.Parallel()
		desc := fake.Lorem().Sentence(6)
		content := "---\ndescription: " + desc + "\n---\nbody"
		_, err := parseSkillFile([]byte(content), "my-skill")
		require.Error(t, err)
		assert.ErrorContains(t, err, "name")
	})

	t.Run("missing_description_field_returns_error", func(t *testing.T) {
		t.Parallel()
		content := "---\nname: my-skill\n---\nbody"
		_, err := parseSkillFile([]byte(content), "my-skill")
		require.Error(t, err)
		assert.ErrorContains(t, err, "description")
	})

	t.Run("name_invalid_format_returns_error", func(t *testing.T) {
		t.Parallel()
		desc := fake.Lorem().Sentence(6)
		content := "---\nname: My_Skill!\ndescription: " + desc + "\n---\nbody"
		_, err := parseSkillFile([]byte(content), "My_Skill!")
		require.Error(t, err)
		assert.ErrorContains(t, err, "name")
	})

	t.Run("name_too_long_returns_error", func(t *testing.T) {
		t.Parallel()
		longName := strings.Repeat("a", 65)
		desc := fake.Lorem().Sentence(6)
		content := "---\nname: " + longName + "\ndescription: " + desc + "\n---\nbody"
		_, err := parseSkillFile([]byte(content), longName)
		require.Error(t, err)
		assert.ErrorContains(t, err, "name")
	})

	t.Run("name_max_length_64_is_valid", func(t *testing.T) {
		t.Parallel()
		name := strings.Repeat("a", 64)
		desc := fake.Lorem().Sentence(6)
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody"
		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Equal(t, name, got.Name)
	})

	t.Run("name_does_not_match_dir_name_returns_error", func(t *testing.T) {
		t.Parallel()
		desc := fake.Lorem().Sentence(6)
		content := "---\nname: my-skill\ndescription: " + desc + "\n---\nbody"
		_, err := parseSkillFile([]byte(content), "other-name")
		require.Error(t, err)
		assert.ErrorContains(t, err, "name")
	})

	t.Run("description_too_long_returns_error", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		longDesc := strings.Repeat("x", 1025)
		content := "---\nname: " + name + "\ndescription: " + longDesc + "\n---\nbody"
		_, err := parseSkillFile([]byte(content), name)
		require.Error(t, err)
		assert.ErrorContains(t, err, "description")
	})

	t.Run("description_max_length_1024_is_valid", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		desc := strings.Repeat("x", 1024)
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody"
		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Equal(t, desc, got.Description)
	})

	t.Run("body_is_empty_string_when_no_content_after_frontmatter", func(t *testing.T) {
		t.Parallel()
		name := "my-skill"
		desc := fake.Lorem().Sentence(6)
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n"

		got, err := parseSkillFile([]byte(content), name)
		require.NoError(t, err)
		assert.Empty(t, got.Body)
	})
}
