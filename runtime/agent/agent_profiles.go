package agent

import (
	"context"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ErrAgentProfileNotFound is returned when a profile with the given name does not exist.
var ErrAgentProfileNotFound = ap.ErrAgentProfileNotFound

// ErrAgentProfileNameConflict is returned when a profile with the given name already exists.
var ErrAgentProfileNameConflict = ap.ErrAgentProfileNameConflict

// ExecutionSettings stores runtime-owned execution defaults for a profile.
type ExecutionSettings = ap.ExecutionSettings

// AgentProfile is a persisted general-purpose agent profile definition.
type AgentProfile = ap.AgentProfile

// CreateAgentProfileParams contains parameters required to create a profile.
type CreateAgentProfileParams = ap.CreateAgentProfileParams

// UpdateAgentProfileParams contains mutable parameters for profile updates.
type UpdateAgentProfileParams = ap.UpdateAgentProfileParams

// AgentProfilesService manages persisted agent profiles.
type AgentProfilesService interface {
	List(ctx context.Context) ([]AgentProfile, error)
	Get(ctx context.Context, name string) (*AgentProfile, error)
	Create(ctx context.Context, params CreateAgentProfileParams) (*AgentProfile, error)
	Update(ctx context.Context, name string, params UpdateAgentProfileParams) (*AgentProfile, error)
	Delete(ctx context.Context, name string) error
	AutoMigrate() error
}
