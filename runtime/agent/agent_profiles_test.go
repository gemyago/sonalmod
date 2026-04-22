//go:build !release

package agent

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal"
	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/stretchr/testify/require"
)

func TestAgentProfilesAliases(t *testing.T) {
	require.ErrorIs(t, ErrAgentProfileNotFound, ap.ErrAgentProfileNotFound)
	require.ErrorIs(t, ErrAgentProfileNameConflict, ap.ErrAgentProfileNameConflict)
}

func TestNewFileAgentProfilesService(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()
	svc, err := NewFileAgentProfilesService(t.TempDir(), rootTestLogger)
	require.NoError(t, err)
	require.NotNil(t, svc)
}

func TestNewDatabaseAgentProfilesService(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()
	svc, err := NewDatabaseAgentProfilesService(":memory:", rootTestLogger, "")
	require.NoError(t, err)
	require.NotNil(t, svc)
}
