package firecrawl

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
)

type captureRegistry struct {
	addCalls int
	tools    []agent.DefinedTool
}

func (c *captureRegistry) AddTools(tools ...agent.DefinedTool) {
	c.addCalls++
	c.tools = append(c.tools, tools...)
}

func TestRegisterTools(t *testing.T) {
	fake := faker.New()

	type registryState struct {
		addCalls  int
		toolCount int
	}

	makeMockDeps := func() *captureRegistry {
		return &captureRegistry{}
	}

	t.Run("skips_AddTools_when_base_URL_missing_or_whitespace", func(t *testing.T) {
		tests := []struct {
			name string
			opts []RegisterToolsOpt
		}{
			{
				name: "missing",
				opts: []RegisterToolsOpt{WithAuthToken(fake.Lorem().Word())},
			},
			{
				name: "whitespace_only",
				opts: []RegisterToolsOpt{WithBaseURL(strings.Repeat(" ", fake.IntBetween(1, 4)) + "\t")},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reg := makeMockDeps()
				RegisterTools(reg, tt.opts...)
				assert.Equal(t, registryState{}, registryState{reg.addCalls, len(reg.tools)})
			})
		}
	})

	t.Run("AddTools_registers_two_tools_when_base_URL_configured", func(t *testing.T) {
		reg := makeMockDeps()
		baseURL := "https://" + fake.Internet().Domain()
		RegisterTools(reg,
			WithBaseURL(baseURL),
			WithAuthToken(fake.UUID().V4()),
			WithClientLogger(slog.New(slog.DiscardHandler)),
			WithClientHTTPClient(http.DefaultClient),
		)
		want := registryState{addCalls: 1, toolCount: 2}
		got := registryState{reg.addCalls, len(reg.tools)}
		assert.Equal(t, want, got)
	})
}
