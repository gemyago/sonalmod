//go:build !release

package httpapi

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/gemyago/sonalmod/runtime/internal"
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
	newTestProfileRunDispatcher := func(t *testing.T) agent.ProfileRunDispatcher {
		t.Helper()
		dispatcher, err := agent.NewProfileRunDispatcher(newTestProfilesService(t), newTestRunner(t))
		require.NoError(t, err)
		return dispatcher
	}

	t.Run("creates handler", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
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
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: nil,
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "providers config service is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if AgentProfilesService is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   nil,
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "agent profiles service is required")
		assert.Nil(t, handler)
	})

	t.Run("ignores OpenCodeBindingService when omitted", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})

	t.Run("ignores OpenCodeLauncher when omitted", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})

	t.Run("creates handler with non-nil services", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   newTestProfileRunDispatcher(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})

	t.Run("returns error if ProfileRunDispatcher is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProfileRunDispatcher:   nil,
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "profile run dispatcher is required")
		assert.Nil(t, handler)
	})
}
