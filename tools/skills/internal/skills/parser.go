package skills

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

const (
	maxNameLength        = 64
	maxDescriptionLength = 1024
)

var namePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// SkillEntry holds parsed metadata and body for a single skill.
type SkillEntry struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	AllowedTools  string
	Body          string
}

type skillFrontmatter struct {
	Name          string `yaml:"name"`
	Description   string `yaml:"description"`
	License       string `yaml:"license"`
	Compatibility string `yaml:"compatibility"`
	AllowedTools  string `yaml:"allowed-tools"`
}

// parseSkillFile parses a SKILL.md file given its content and the expected skill
// name (derived from the parent directory name). Returns a SkillEntry or an error
// if the content is invalid.
func parseSkillFile(content []byte, dirName string) (SkillEntry, error) {
	const delimiter = "---"

	// Content must start with "---\n"
	if !bytes.HasPrefix(content, []byte(delimiter+"\n")) {
		return SkillEntry{}, errors.New("skills: missing frontmatter delimiter")
	}

	// Find the closing "---" delimiter
	rest := content[len(delimiter)+1:]
	idx := bytes.Index(rest, []byte("\n"+delimiter))
	if idx < 0 {
		return SkillEntry{}, errors.New("skills: unclosed frontmatter delimiter")
	}

	frontmatterBytes := rest[:idx]
	afterDelimiter := rest[idx+1+len(delimiter):]

	// Extract body: skip the leading newline after the closing delimiter
	body := ""
	if len(afterDelimiter) > 0 && afterDelimiter[0] == '\n' {
		body = string(afterDelimiter[1:])
	} else if len(afterDelimiter) > 0 {
		body = string(afterDelimiter)
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal(frontmatterBytes, &fm); err != nil {
		return SkillEntry{}, fmt.Errorf("skills: malformed frontmatter: %w", err)
	}

	if err := validateFrontmatter(fm, dirName); err != nil {
		return SkillEntry{}, err
	}

	return SkillEntry{
		Name:          fm.Name,
		Description:   fm.Description,
		License:       fm.License,
		Compatibility: fm.Compatibility,
		AllowedTools:  fm.AllowedTools,
		Body:          body,
	}, nil
}

func validateFrontmatter(fm skillFrontmatter, dirName string) error {
	if fm.Name == "" {
		return errors.New("skills: name is required")
	}
	if len(fm.Name) > maxNameLength {
		return fmt.Errorf("skills: name exceeds maximum length of %d", maxNameLength)
	}
	if !namePattern.MatchString(fm.Name) {
		return fmt.Errorf("skills: name %q has invalid format (must match %s)", fm.Name, namePattern.String())
	}
	if fm.Name != dirName {
		return fmt.Errorf("skills: name %q must match parent directory name %q", fm.Name, dirName)
	}
	if fm.Description == "" {
		return errors.New("skills: description is required")
	}
	if len(fm.Description) > maxDescriptionLength {
		return fmt.Errorf("skills: description exceeds maximum length of %d", maxDescriptionLength)
	}
	return nil
}
