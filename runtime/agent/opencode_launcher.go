package agent

import (
	"context"

	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
)

// OpenCodeLaunchErrorKind classifies launcher failures for API-level mapping.
type OpenCodeLaunchErrorKind = cl.OpenCodeLaunchErrorKind

const (
	// OpenCodeLaunchErrorKindValidation indicates request/config validation failure.
	OpenCodeLaunchErrorKindValidation = cl.OpenCodeLaunchErrorKindValidation
	// OpenCodeLaunchErrorKindNotFound indicates missing profile or binding state.
	OpenCodeLaunchErrorKindNotFound = cl.OpenCodeLaunchErrorKindNotFound
	// OpenCodeLaunchErrorKindLaunchFailed indicates subprocess/protocol execution failure.
	OpenCodeLaunchErrorKindLaunchFailed = cl.OpenCodeLaunchErrorKindLaunchFailed
)

// OpenCodeLaunchError wraps launcher failures with a stable kind and operation.
type OpenCodeLaunchError = cl.OpenCodeLaunchError

// OpenCodeLaunchRequest identifies saved config and a prompt for a coding run.
type OpenCodeLaunchRequest = cl.OpenCodeLaunchRequest

// OpenCodeLaunchResult contains resolved launch identifiers and ACP output.
type OpenCodeLaunchResult = cl.OpenCodeLaunchResult

// OpenCodeLauncher launches OpenCode coding runs from saved selectors.
type OpenCodeLauncher interface {
	Launch(ctx context.Context, request OpenCodeLaunchRequest) (*OpenCodeLaunchResult, error)
}

// NewOpenCodeLauncher creates an OpenCode launcher with the default ACP client.
func NewOpenCodeLauncher( //nolint:ireturn // public contract returns launcher interface.
	profiles AgentProfilesService,
	bindings OpenCodeBindingService,
) (OpenCodeLauncher, error) {
	return cl.NewOpenCodeACPLauncher(profiles, bindings, cl.NewOpenCodeACPClient())
}
