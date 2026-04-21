package agent

import (
	"encoding/json"
	"fmt"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ToolDef is a generic tool definition for LLM agents with typed args and results.
type ToolDef[TArgs, TResults any] struct {
	Name        string
	Description string
	Handler     func(ctx *ToolContext, input TArgs) (TResults, error)
	// rawOutputSchema holds an optional JSON schema override for the output type.
	// When set, schema inference from TResults is skipped (useful for recursive types).
	rawOutputSchema []byte
}

func NewToolDef[TArgs, TResults any](
	name, description string,
	handler func(ctx *ToolContext, input TArgs) (TResults, error),
) ToolDef[TArgs, TResults] {
	return ToolDef[TArgs, TResults]{
		Name:        name,
		Description: description,
		Handler:     handler,
	}
}

func (t ToolDef[TArgs, TResults]) name() string {
	return t.Name
}

// WithOutputSchema returns a copy of the tool definition with a custom output schema
// provided as raw JSON bytes. Use this when the output type cannot be inferred
// automatically (e.g., recursive types that would cause cycle-detection errors).
func (t ToolDef[TArgs, TResults]) WithOutputSchema(schemaJSON []byte) ToolDef[TArgs, TResults] {
	t.rawOutputSchema = schemaJSON
	return t
}

func (t ToolDef[TArgs, TResults]) handleADK(adkCtx tool.Context, in TArgs) (TResults, error) { //nolint:ireturn
	return t.Handler(newToolContextFromADK(adkCtx), in)
}

// newADKTool creates an ADK tool from the LLMTool.
func (t ToolDef[TArgs, TResults]) newADKTool() (tool.Tool, error) { //nolint:ireturn
	cfg := functiontool.Config{
		Name:        t.Name,
		Description: t.Description,
	}
	if len(t.rawOutputSchema) > 0 {
		var schema jsonschema.Schema
		if err := json.Unmarshal(t.rawOutputSchema, &schema); err != nil {
			return nil, fmt.Errorf("failed to create ADK tool %s: invalid output schema JSON: %w", t.Name, err)
		}
		cfg.OutputSchema = &schema
	}
	adkTool, err := functiontool.New(cfg, t.handleADK)
	if err != nil {
		return nil, fmt.Errorf("failed to create ADK tool %s: %w", t.Name, err)
	}
	return adkTool, nil
}

// defineGenkitToolStub defines a Genkit tool stub for the LLMTool.
// Genkit will delegate tool calls to the ADK but we still need to provide schema
// to the genkit since we use genkit for LLM execution.
func (t ToolDef[TArgs, TResults]) defineGenkitToolStub(g *genkit.Genkit) {
	genkit.DefineTool(g, t.Name, t.Description,
		func(_ *ai.ToolContext, _ TArgs) (TResults, error) { // coverage-ignore // this is a stub that is never invoked
			var zero TResults
			return zero, fmt.Errorf("tool %s is a stub and should not be called", t.Name)
		},
	)
}
