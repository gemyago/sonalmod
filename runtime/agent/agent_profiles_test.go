//go:build !release

package agent

import (
	"testing"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
	"github.com/stretchr/testify/assert"
)

func TestAgentProfilesAliases(t *testing.T) {
	assert.ErrorIs(t, ErrAgentProfileNotFound, ap.ErrAgentProfileNotFound)
	assert.ErrorIs(t, ErrAgentProfileNameConflict, ap.ErrAgentProfileNameConflict)

	var _ AgentProfilesService = (ap.AgentProfilesService)(nil)
}
