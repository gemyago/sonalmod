//go:build !release

package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/gemyago/sonalmod/runtime/internal"
	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()
	newTestRunner := func(t *testing.T) *agent.Runner {
		t.Helper()
		runner, err := agent.NewRunner(agent.RunnerArgs{
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
		}, agent.WithLogger(rootTestLogger))
		require.NoError(t, err)
		return runner
	}
	newTestProfilesService := func(t *testing.T) agent.AgentProfilesService {
		t.Helper()
		svc, err := agent.NewFileAgentProfilesService(t.TempDir(), rootTestLogger)
		require.NoError(t, err)
		return svc
	}
	newTestOpenCodeBindingService := func(t *testing.T) agent.OpenCodeBindingService {
		t.Helper()
		svc, err := agent.NewFileOpenCodeBindingService(t.TempDir(), rootTestLogger)
		require.NoError(t, err)
		return svc
	}
	newTestOpenCodeLauncher := func(t *testing.T) agent.OpenCodeLauncher {
		t.Helper()
		profilesSvc := newTestProfilesService(t)
		bindingsSvc := newTestOpenCodeBindingService(t)
		profile, err := profilesSvc.Create(t.Context(), agent.CreateAgentProfileParams{
			Name:         "profile-main",
			Role:         "coder",
			Instructions: "help",
			ExecutionSettings: agent.ExecutionSettings{
				DefaultModel: "openai/gpt-4.1",
			},
		})
		require.NoError(t, err)
		_, err = bindingsSvc.Create(t.Context(), agent.CreateOpenCodeBindingParams{
			Name:        "binding-main",
			ProfileName: profile.Name,
			AgentCommand: agent.OpenCodeAgentCommand{
				Command: "echo",
				Args:    []string{"stub"},
			},
			LaunchOptions: agent.OpenCodeLaunchOptions{Transport: "stdio"},
		})
		require.NoError(t, err)

		launcher, err := cl.NewOpenCodeACPLauncher(profilesSvc, bindingsSvc, &stubACPClient{})
		require.NoError(t, err)
		return launcher
	}

	t.Run("creates handler", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
			OpenCodeBindingService: newTestOpenCodeBindingService(t),
			OpenCodeLauncher:       newTestOpenCodeLauncher(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})

	t.Run("returns error if runner is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner: nil,
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "runner is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if ProvidersConfigService is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: nil,
			AgentProfilesService:   newTestProfilesService(t),
			OpenCodeBindingService: newTestOpenCodeBindingService(t),
			OpenCodeLauncher:       newTestOpenCodeLauncher(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "providers config service is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if AgentProfilesService is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   nil,
			OpenCodeBindingService: newTestOpenCodeBindingService(t),
			OpenCodeLauncher:       newTestOpenCodeLauncher(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "agent profiles service is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if OpenCodeBindingService is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
			OpenCodeBindingService: nil,
			OpenCodeLauncher:       newTestOpenCodeLauncher(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "opencode binding service is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if OpenCodeLauncher is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
			OpenCodeBindingService: newTestOpenCodeBindingService(t),
			OpenCodeLauncher:       nil,
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "opencode launcher is required")
		assert.Nil(t, handler)
	})

	t.Run("creates handler with non-nil services", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
			OpenCodeBindingService: newTestOpenCodeBindingService(t),
			OpenCodeLauncher:       newTestOpenCodeLauncher(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})
}

type stubACPClient struct{}

func (s *stubACPClient) Launch(
	_ context.Context,
	_ cl.OpenCodeACPLaunchRequest,
) (*cl.OpenCodeACPLaunchResult, error) {
	result := map[string]any{"status": "ok"}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &cl.OpenCodeACPLaunchResult{
		SessionID:    "session-main",
		PromptResult: encoded,
		Updates:      []cl.OpenCodeACPUpdate{},
	}, nil
}
