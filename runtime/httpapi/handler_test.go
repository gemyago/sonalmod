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
	newTestProfilesService := func(t *testing.T) agent.AgentProfilesService {
		t.Helper()
		svc, err := agent.NewFileAgentProfilesService(t.TempDir(), rootTestLogger)
		require.NoError(t, err)
		return svc
	}
	newTestRunner := func(t *testing.T) *agent.Runner {
		t.Helper()
		runner, err := agent.NewRunner(agent.RunnerArgs{
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, agent.WithLogger(rootTestLogger))
		require.NoError(t, err)
		return runner
	}

	t.Run("creates handler", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
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
			ProvidersConfigService: nil,
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "providers config service is required")
		assert.Nil(t, handler)
	})

	t.Run("returns error if AgentProfilesService is nil", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   nil,
		}, WithLogger(rootTestLogger))
		require.ErrorContains(t, err, "agent profiles service is required")
		assert.Nil(t, handler)
	})

	t.Run("creates handler with non-nil services", func(t *testing.T) {
		handler, err := NewHandler(HandlerArgs{
			Runner:                 newTestRunner(t),
			ProvidersConfigService: lp.NewMockProvidersConfigService(t),
			AgentProfilesService:   newTestProfilesService(t),
		}, WithLogger(rootTestLogger))
		require.NoError(t, err)
		require.NotNil(t, handler)
	})
}
