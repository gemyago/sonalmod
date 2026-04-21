package agent

import (
	"fmt"

	"github.com/firebase/genkit/go/genkit"
	adktool "google.golang.org/adk/tool"
)

type DefinedTool interface {
	name() string
	newADKTool() (adktool.Tool, error)
	defineGenkitToolStub(g *genkit.Genkit)
}

// ToolsRegistry allows to register tools available to the agent.
type ToolsRegistry struct {
	tools []DefinedTool
}

// NewToolsRegistry creates a new ToolsRegistry.
func NewToolsRegistry() *ToolsRegistry {
	return &ToolsRegistry{
		tools: nil,
	}
}

func (r *ToolsRegistry) AddTools(tools ...DefinedTool) {
	r.tools = append(r.tools, tools...)
}

// defineGenkitToolStubs registers Genkit tool stubs for each defined tool on g.
// Kept package-private so the public contract does not expose *genkit.Genkit.
func (r *ToolsRegistry) defineGenkitToolStubs(g *genkit.Genkit) {
	for _, tool := range r.tools {
		tool.defineGenkitToolStub(g)
	}
}

// toolsRegistryProvider implements internal.ToolsProvider: builds ADK tools from a
// registry without exporting GetTools on ToolsRegistry.
type toolsRegistryProvider struct {
	reg *ToolsRegistry
}

func (p *toolsRegistryProvider) GetTools() ([]adktool.Tool, error) {
	reg := p.reg
	tools := make([]adktool.Tool, 0, len(reg.tools))
	for _, dt := range reg.tools {
		t, err := dt.newADKTool()
		if err != nil {
			return nil, fmt.Errorf("failed to create ADK tool %s: %w", dt.name(), err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}
