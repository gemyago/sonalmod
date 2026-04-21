package main

import (
	"errors"

	"github.com/gemyago/sonalmod/runtime/agent"
)

// toolsRegistrar is satisfied by *agent.ToolsRegistry.
type toolsRegistrar interface {
	AddTools(tools ...agent.DefinedTool)
}

const (
	testLocationLatitude  = 40.7128
	testLocationLongitude = -74.0060
	testWeatherTemp       = 22.5
	testWeatherHumidity   = 65
)

type locationResult struct {
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type weatherInput struct {
	Location string `json:"location"`
}

type weatherResult struct {
	Location    string  `json:"location"`
	Temperature float64 `json:"temperature"`
	Unit        string  `json:"unit"`
	Conditions  string  `json:"conditions"`
	Humidity    int     `json:"humidity"`
}

func newGetLocationTool() agent.ToolDef[struct{}, locationResult] {
	return agent.NewToolDef[struct{}, locationResult](
		"test_get_location",
		"Returns the current location. For testing purposes only.",
		func(_ *agent.ToolContext, _ struct{}) (locationResult, error) {
			return locationResult{
				City:      "New York",
				Country:   "US",
				Latitude:  testLocationLatitude,
				Longitude: testLocationLongitude,
			}, nil
		},
	)
}

func newGetWeatherTool() agent.ToolDef[weatherInput, weatherResult] {
	return agent.NewToolDef[weatherInput, weatherResult](
		"test_get_weather",
		"Returns weather conditions for a given location. For testing purposes only.",
		func(_ *agent.ToolContext, input weatherInput) (weatherResult, error) {
			if input.Location == "" {
				return weatherResult{}, errors.New("test_get_weather: location is required")
			}
			return weatherResult{
				Location:    input.Location,
				Temperature: testWeatherTemp,
				Unit:        "celsius",
				Conditions:  "Partly Cloudy",
				Humidity:    testWeatherHumidity,
			}, nil
		},
	)
}

// registerTestTools adds dummy test tools to the registry.
func registerTestTools(registry toolsRegistrar) {
	registry.AddTools(
		newGetLocationTool(),
		newGetWeatherTool(),
	)
}
