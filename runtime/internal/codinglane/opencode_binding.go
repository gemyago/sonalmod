package codinglane

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

var opencodeBindingNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ErrOpenCodeBindingNotFound is returned when a binding with the given name does not exist.
var ErrOpenCodeBindingNotFound = errors.New("opencode binding not found")

// ErrOpenCodeBindingNameConflict is returned when a binding with the given name already exists.
var ErrOpenCodeBindingNameConflict = errors.New("opencode binding name already exists")

// OpenCodeAgentCommand stores command defaults used to launch OpenCode ACP.
type OpenCodeAgentCommand struct {
	Command string
	Args    []string
}

// OpenCodeLaunchOptions stores backend-specific OpenCode launch defaults.
type OpenCodeLaunchOptions struct {
	Transport string
}

// OpenCodeBinding links a general profile to OpenCode connection defaults.
type OpenCodeBinding struct {
	Name          string
	ProfileName   string
	CWD           string
	AgentCommand  OpenCodeAgentCommand
	LaunchOptions OpenCodeLaunchOptions
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CreateOpenCodeBindingParams contains parameters required to create a binding.
type CreateOpenCodeBindingParams struct {
	Name          string
	ProfileName   string
	CWD           string
	AgentCommand  OpenCodeAgentCommand
	LaunchOptions OpenCodeLaunchOptions
}

// UpdateOpenCodeBindingParams contains mutable parameters for binding updates.
type UpdateOpenCodeBindingParams struct {
	CWD           string
	AgentCommand  OpenCodeAgentCommand
	LaunchOptions OpenCodeLaunchOptions
}

// OpenCodeBindingService manages persisted OpenCode binding definitions.
type OpenCodeBindingService interface {
	// List returns all bindings sorted by CreatedAt ascending.
	List(ctx context.Context) ([]OpenCodeBinding, error)

	// Get returns a binding by name.
	// Returns ErrOpenCodeBindingNotFound when no binding exists with this name.
	Get(ctx context.Context, name string) (*OpenCodeBinding, error)

	// Create creates a new binding.
	// Returns ErrOpenCodeBindingNameConflict when a binding already exists with this name.
	Create(ctx context.Context, params CreateOpenCodeBindingParams) (*OpenCodeBinding, error)

	// Update modifies mutable fields of the binding identified by name.
	// Returns ErrOpenCodeBindingNotFound when no binding exists with this name.
	Update(ctx context.Context, name string, params UpdateOpenCodeBindingParams) (*OpenCodeBinding, error)

	// Delete removes a binding by name.
	// Returns ErrOpenCodeBindingNotFound when no binding exists with this name.
	Delete(ctx context.Context, name string) error

	// AutoMigrate applies required persistence migrations for the current backend.
	AutoMigrate() error
}

func normalizeCreateOpenCodeBindingParams(
	params CreateOpenCodeBindingParams,
) (CreateOpenCodeBindingParams, error) {
	name := strings.TrimSpace(params.Name)
	if !opencodeBindingNamePattern.MatchString(name) {
		return CreateOpenCodeBindingParams{}, fmt.Errorf(
			"invalid binding name %q: must match ^[a-z][a-z0-9-]*$",
			params.Name,
		)
	}

	profileName := strings.TrimSpace(params.ProfileName)
	if profileName == "" {
		return CreateOpenCodeBindingParams{}, errors.New("profile_name is required")
	}
	if !opencodeBindingNamePattern.MatchString(profileName) {
		return CreateOpenCodeBindingParams{}, fmt.Errorf(
			"invalid profile name %q: must match ^[a-z][a-z0-9-]*$",
			params.ProfileName,
		)
	}

	normalizedCommand, err := normalizeAgentCommand(params.AgentCommand)
	if err != nil {
		return CreateOpenCodeBindingParams{}, err
	}

	normalizedOptions, err := normalizeLaunchOptions(params.LaunchOptions)
	if err != nil {
		return CreateOpenCodeBindingParams{}, err
	}

	params.Name = name
	params.ProfileName = profileName
	params.CWD = strings.TrimSpace(params.CWD)
	params.AgentCommand = normalizedCommand
	params.LaunchOptions = normalizedOptions
	return params, nil
}

func applyOpenCodeBindingUpdate(
	existing OpenCodeBinding,
	params UpdateOpenCodeBindingParams,
) (OpenCodeBinding, error) {
	normalizedCommand, err := normalizeAgentCommand(params.AgentCommand)
	if err != nil {
		return OpenCodeBinding{}, err
	}

	normalizedOptions, err := normalizeLaunchOptions(params.LaunchOptions)
	if err != nil {
		return OpenCodeBinding{}, err
	}

	updated := existing
	updated.CWD = strings.TrimSpace(params.CWD)
	updated.AgentCommand = normalizedCommand
	updated.LaunchOptions = normalizedOptions
	return updated, nil
}

func normalizeAgentCommand(command OpenCodeAgentCommand) (OpenCodeAgentCommand, error) {
	command.Command = strings.TrimSpace(command.Command)
	if command.Command == "" {
		return OpenCodeAgentCommand{}, errors.New("agent_command.command is required")
	}
	if strings.ContainsAny(command.Command, "\n\r\t") {
		return OpenCodeAgentCommand{}, errors.New("agent_command.command contains control characters")
	}

	if command.Args == nil {
		command.Args = []string{}
	}
	for idx := range command.Args {
		command.Args[idx] = strings.TrimSpace(command.Args[idx])
		if command.Args[idx] == "" {
			return OpenCodeAgentCommand{}, errors.New("agent_command.args must not contain empty values")
		}
		if strings.ContainsAny(command.Args[idx], "\n\r\t") {
			return OpenCodeAgentCommand{}, errors.New("agent_command.args contain control characters")
		}
	}
	if hasDuplicates(command.Args) {
		return OpenCodeAgentCommand{}, errors.New("agent_command.args must be unique")
	}

	return command, nil
}

func normalizeLaunchOptions(options OpenCodeLaunchOptions) (OpenCodeLaunchOptions, error) {
	options.Transport = strings.TrimSpace(strings.ToLower(options.Transport))
	if options.Transport == "" {
		options.Transport = "stdio"
	}
	if options.Transport != "stdio" {
		return OpenCodeLaunchOptions{}, errors.New("launch_options.transport must be stdio")
	}
	return options, nil
}

func hasDuplicates(values []string) bool {
	seen := make([]string, 0, len(values))
	for _, value := range values {
		if slices.Contains(seen, value) {
			return true
		}
		seen = append(seen, value)
	}
	return false
}
