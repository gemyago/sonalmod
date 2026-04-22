package agent

import (
	"log/slog"

	ap "github.com/gemyago/sonalmod/runtime/internal/agentprofiles"
)

// ErrAgentProfileNotFound is returned when a profile with the given name does not exist.
var ErrAgentProfileNotFound = ap.ErrAgentProfileNotFound

// ErrAgentProfileNameConflict is returned when a profile with the given name already exists.
var ErrAgentProfileNameConflict = ap.ErrAgentProfileNameConflict

// ExecutionSettings stores runtime-owned execution defaults for a profile.
type ExecutionSettings = ap.ExecutionSettings

// AgentProfile is a persisted general-purpose agent profile definition.
//
//nolint:revive // public naming intentionally matches existing runtime contract style.
type AgentProfile = ap.AgentProfile

// CreateAgentProfileParams contains parameters required to create a profile.
type CreateAgentProfileParams = ap.CreateAgentProfileParams

// UpdateAgentProfileParams contains mutable parameters for profile updates.
type UpdateAgentProfileParams = ap.UpdateAgentProfileParams

// AgentProfilesService manages persisted agent profiles.
//
//nolint:revive // public naming intentionally matches existing runtime contract style.
type AgentProfilesService = ap.AgentProfilesService

// NewFileAgentProfilesService creates a file-based [AgentProfilesService] that stores
// profile definitions as YAML files under {baseDir}/agent-profiles/{name}.yaml.
func NewFileAgentProfilesService( //nolint:ireturn
	baseDir string,
	logger *slog.Logger,
) (AgentProfilesService, error) {
	return ap.NewFileAgentProfilesService(baseDir, logger)
}

// NewDatabaseAgentProfilesService creates a database-backed [AgentProfilesService] that stores
// profile definitions in a relational database identified by the given DSN.
// tablePrefix sets the prefix for persisted SQL table names; empty means no prefix.
func NewDatabaseAgentProfilesService( //nolint:ireturn
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (AgentProfilesService, error) {
	return ap.NewDatabaseAgentProfilesService(dsn, logger, tablePrefix)
}
