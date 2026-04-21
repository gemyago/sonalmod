package skills

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gemyago/sonalmod/runtime/agent"
	iskills "github.com/gemyago/sonalmod/tools/skills/internal/skills"
)

// ExpectedToolCount is the number of skill tools registered by Skills.RegisterTools.
const ExpectedToolCount = 2

// ToolsRegistry is the minimal surface needed to register agent tools.
type ToolsRegistry interface {
	AddTools(tools ...agent.DefinedTool)
}

// Skills holds a built skill catalog and is the integration point for tool registration
// and system prompt fragments. Construct with [New].
type Skills struct {
	cat    *iskills.Catalog
	logger *slog.Logger
}

type skillsConfig struct {
	logger      *slog.Logger
	iskillsOpts []iskills.CatalogOption
}

// Option configures [New].
type Option func(*skillsConfig)

// WithLogger sets the logger used when building the catalog and for tool registration.
// Nil falls back to [slog.Default] (same as omitting [WithLogger]).
func WithLogger(logger *slog.Logger) Option {
	return func(c *skillsConfig) {
		c.logger = logger
		c.iskillsOpts = append(c.iskillsOpts, iskills.WithLogger(logger))
	}
}

// WithMaxSkillBytes sets the maximum number of bytes allowed per SKILL.md file.
func WithMaxSkillBytes(n int) Option {
	return func(c *skillsConfig) {
		c.iskillsOpts = append(c.iskillsOpts, iskills.WithMaxSkillBytes(n))
	}
}

// WithMaxCatalogEntries sets the maximum number of skills in the catalog.
func WithMaxCatalogEntries(n int) Option {
	return func(c *skillsConfig) {
		c.iskillsOpts = append(c.iskillsOpts, iskills.WithMaxCatalogEntries(n))
	}
}

// New builds a [Skills] value by scanning the given skill roots.
// Non-existent roots are skipped with a warning log.
// Invalid or oversized SKILL.md files are skipped with a warning log.
// Duplicate skill names are resolved by root order (first wins).
// When no [WithLogger] option is set, logging uses [slog.Default].
func New(roots []string, opts ...Option) (*Skills, error) {
	cfg := &skillsConfig{}
	for _, o := range opts {
		o(cfg)
	}
	cat, err := iskills.NewCatalog(roots, cfg.iskillsOpts...)
	if err != nil {
		return nil, err
	}
	logger := cfg.logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Skills{cat: cat, logger: logger}, nil
}

// RegisterTools registers skill tools (skills_list, skills_read) with the registry.
func (s *Skills) RegisterTools(registry ToolsRegistry) {
	registry.AddTools(skillsAgentTools(s.cat, s.logger)...)
}

// BuildSystemPromptFragments returns zero or one Skills section for use with agent.WithSystemPromptFragments
// (spread into the variadic). When the catalog is empty, returns nil.
func (s *Skills) BuildSystemPromptFragments() []agent.SystemPromptFragment {
	entries := s.cat.List()
	if len(entries) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "<skill name=%q description=%q />\n", e.Name, e.Description)
	}
	sb.WriteString("</available_skills>")
	return []agent.SystemPromptFragment{{
		Section: "Skills",
		Content: sb.String(),
	}}
}
