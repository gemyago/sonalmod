package main

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureToolsRegistry captures tools added via AddTools.
type captureToolsRegistry struct {
	tools []agent.DefinedTool
}

func (c *captureToolsRegistry) AddTools(tools ...agent.DefinedTool) {
	c.tools = append(c.tools, tools...)
}

func TestRegisterTestTools(t *testing.T) {
	t.Parallel()

	t.Run("adds_exactly_two_tools", func(t *testing.T) {
		t.Parallel()
		reg := &captureToolsRegistry{}
		registerTestTools(reg)
		assert.Len(t, reg.tools, 2)
	})
}

func TestTestGetLocationTool(t *testing.T) {
	t.Parallel()

	toolCtx := &agent.ToolContext{Context: t.Context()}

	t.Run("returns_expected_location", func(t *testing.T) {
		t.Parallel()
		tool := newGetLocationTool()
		result, err := tool.Handler(toolCtx, struct{}{})
		require.NoError(t, err)
		assert.Equal(t, locationResult{
			City:      "New York",
			Country:   "US",
			Latitude:  testLocationLatitude,
			Longitude: testLocationLongitude,
		}, result)
	})
}

func TestTestGetWeatherTool(t *testing.T) {
	t.Parallel()

	toolCtx := &agent.ToolContext{Context: t.Context()}

	t.Run("returns_expected_weather_for_location", func(t *testing.T) {
		t.Parallel()
		tool := newGetWeatherTool()
		result, err := tool.Handler(toolCtx, weatherInput{Location: "New York"})
		require.NoError(t, err)
		assert.Equal(t, weatherResult{
			Location:    "New York",
			Temperature: testWeatherTemp,
			Unit:        "celsius",
			Conditions:  "Partly Cloudy",
			Humidity:    testWeatherHumidity,
		}, result)
	})

	t.Run("returns_error_for_empty_location", func(t *testing.T) {
		t.Parallel()
		tool := newGetWeatherTool()
		_, err := tool.Handler(toolCtx, weatherInput{Location: ""})
		require.Error(t, err)
	})
}
