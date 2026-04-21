package agent

import (
	"testing"

	"github.com/firebase/genkit/go/genkit"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/tool"
)

func TestToolsRegistry(t *testing.T) {
	fake := faker.New()
	toolsFromRegistry := func(reg *ToolsRegistry) ([]tool.Tool, error) {
		return (&toolsRegistryProvider{reg: reg}).GetTools()
	}

	t.Run("NewToolsRegistry", func(t *testing.T) {
		registry := NewToolsRegistry()
		require.NotNil(t, registry)
		tools, err := toolsFromRegistry(registry)
		require.NoError(t, err)
		assert.Empty(t, tools)
	})

	t.Run("AddTools_multipleConstructors", func(t *testing.T) {
		ctx := t.Context()
		g := genkit.Init(ctx)
		require.NotNil(t, g)

		c1 := NewToolDef[struct{ In string }, struct{ Out string }](
			"tool_a_"+fake.Lorem().Word(),
			fake.Lorem().Sentence(2),
			func(_ *ToolContext, _ struct{ In string }) (struct{ Out string }, error) {
				return struct{ Out string }{}, nil
			},
		)
		c2 := NewToolDef[struct{ In string }, struct{ Out string }](
			"tool_b_"+fake.Lorem().Word(),
			fake.Lorem().Sentence(2),
			func(_ *ToolContext, _ struct{ In string }) (struct{ Out string }, error) {
				return struct{ Out string }{}, nil
			},
		)

		registry := NewToolsRegistry()
		registry.AddTools(c1, c2)
		registry.defineGenkitToolStubs(g)

		tools, err := toolsFromRegistry(registry)
		require.NoError(t, err)
		require.Len(t, tools, 2)
		assert.Equal(t, c1.name(), tools[0].Name())
		assert.Equal(t, c2.name(), tools[1].Name())
	})

	t.Run("GetTools_constructorError", func(t *testing.T) {
		ctx := t.Context()
		g := genkit.Init(ctx)
		require.NotNil(t, g)

		toolName := "failing_" + fake.Lorem().Word()
		mockConstructor := NewMockDefinedTool(t)
		mockConstructor.EXPECT().defineGenkitToolStub(g)
		mockConstructor.EXPECT().newADKTool().Return(nil, assert.AnError)
		mockConstructor.EXPECT().name().Return(toolName)

		registry := NewToolsRegistry()
		registry.AddTools(mockConstructor)
		registry.defineGenkitToolStubs(g)

		tools, err := toolsFromRegistry(registry)
		require.Error(t, err)
		assert.Nil(t, tools)
		assert.Contains(t, err.Error(), "failed to create ADK tool")
		assert.Contains(t, err.Error(), toolName)
	})

	t.Run("GetTools_returnsConsistentTools", func(t *testing.T) {
		ctx := t.Context()
		g := genkit.Init(ctx)
		require.NotNil(t, g)

		constructor := ToolDef[struct{ X string }, struct{ Y string }]{
			Name:        "get_tools_" + fake.Lorem().Word(),
			Description: fake.Lorem().Sentence(1),
			Handler: func(_ *ToolContext, _ struct{ X string }) (struct{ Y string }, error) {
				return struct{ Y string }{}, nil
			},
		}

		registry := NewToolsRegistry()
		registry.AddTools(constructor)
		registry.defineGenkitToolStubs(g)

		tools1, err := toolsFromRegistry(registry)
		require.NoError(t, err)
		tools2, err := toolsFromRegistry(registry)
		require.NoError(t, err)
		require.Len(t, tools1, 1)
		require.Len(t, tools2, 1)
		assert.Equal(t, tools1[0].Name(), tools2[0].Name())
	})
}
