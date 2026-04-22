package agent

import (
	"log/slog"

	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
)

// ErrOpenCodeBindingNotFound is returned when a binding with the given name does not exist.
var ErrOpenCodeBindingNotFound = cl.ErrOpenCodeBindingNotFound

// ErrOpenCodeBindingNameConflict is returned when a binding with the given name already exists.
var ErrOpenCodeBindingNameConflict = cl.ErrOpenCodeBindingNameConflict

// OpenCodeAgentCommand stores command defaults used to launch OpenCode ACP.
type OpenCodeAgentCommand = cl.OpenCodeAgentCommand

// OpenCodeLaunchOptions stores backend-specific OpenCode launch defaults.
type OpenCodeLaunchOptions = cl.OpenCodeLaunchOptions

// OpenCodeBinding links a general profile to OpenCode connection defaults.
type OpenCodeBinding = cl.OpenCodeBinding

// CreateOpenCodeBindingParams contains parameters required to create a binding.
type CreateOpenCodeBindingParams = cl.CreateOpenCodeBindingParams

// UpdateOpenCodeBindingParams contains mutable parameters for binding updates.
type UpdateOpenCodeBindingParams = cl.UpdateOpenCodeBindingParams

// OpenCodeBindingService manages persisted OpenCode binding definitions.
type OpenCodeBindingService = cl.OpenCodeBindingService

// NewFileOpenCodeBindingService creates a file-backed OpenCode bindings service.
func NewFileOpenCodeBindingService( //nolint:ireturn // public contract returns service interface alias.
	baseDir string,
	logger *slog.Logger,
) (OpenCodeBindingService, error) {
	return cl.NewFileOpenCodeBindingService(baseDir, logger)
}

// NewDatabaseOpenCodeBindingService creates a database-backed OpenCode bindings service.
func NewDatabaseOpenCodeBindingService( //nolint:ireturn // public contract returns service interface alias.
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (OpenCodeBindingService, error) {
	return cl.NewDatabaseOpenCodeBindingService(dsn, logger, tablePrefix)
}
