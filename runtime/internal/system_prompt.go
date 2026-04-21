package internal

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// SystemPromptFragment is a section appended after the base system prompt template.
// The agent package re-exports this type as SystemPromptFragment.
type SystemPromptFragment struct {
	// Section is a short title rendered as a second-level heading.
	Section string
	// Content is the body; may use markdown (use third-level headings for subsections).
	Content string
}

//go:embed system_prompt_base.tmpl
var systemPromptBaseEmbedded string

type systemPromptTemplateData struct {
	AppName string
}

// SystemPromptInstructionProviderOption configures newSystemPromptInstructionProvider.
type SystemPromptInstructionProviderOption func(*systemPromptInstructionProviderConfig)

type systemPromptInstructionProviderConfig struct {
	baseTemplate *string
}

// WithSystemPromptBaseTemplate sets the Go text template used for the base system prompt.
// The default is the embedded system_prompt_base.tmpl. Template data includes .AppName from
// agent.ReadonlyContext. Intended for tests so expectations do not depend on the shipped file.
func WithSystemPromptBaseTemplate(s string) SystemPromptInstructionProviderOption {
	return func(c *systemPromptInstructionProviderConfig) {
		c.baseTemplate = &s
	}
}

func newSystemPromptInstructionProvider(
	fragments []SystemPromptFragment,
	opts ...SystemPromptInstructionProviderOption,
) llmagent.InstructionProvider {
	cfg := systemPromptInstructionProviderConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	baseSrc := systemPromptBaseEmbedded
	if cfg.baseTemplate != nil {
		baseSrc = *cfg.baseTemplate
	}
	sysPromptTmpl := template.Must(template.New("system_prompt_base").Parse(baseSrc))
	return func(rc agent.ReadonlyContext) (string, error) {
		appName := rc.AppName()
		var buf bytes.Buffer
		sysPromptData := systemPromptTemplateData{AppName: appName}
		if err := sysPromptTmpl.Execute(&buf, sysPromptData); err != nil {
			return "", fmt.Errorf("execute system prompt template: %w", err)
		}
		sysPromptBase := strings.TrimSpace(buf.String())
		// Capacity: base template + each fragment + closing heading.
		sections := make([]string, 0, len(fragments)+1+1)
		sections = append(sections, sysPromptBase)
		for _, fragment := range fragments {
			sections = append(sections, fmt.Sprintf("## %s\n\n%s", fragment.Section, fragment.Content))
		}

		// ADK will inject some stuff unconditionally, specifically
		// the identify of the agent, it will be the last thing in the system prompt
		// and it will include something like: "You are an agent. Your internal name is "default".""
		// I'm not sure if it's useful to have it, we will see... For now starting this section
		// and ADK will add it's line in the end.
		sections = append(sections, "## Closing notes")

		return strings.Join(sections, "\n\n"), nil
	}
}
