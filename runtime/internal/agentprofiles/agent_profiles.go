package agentprofiles

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

var agentProfileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ErrAgentProfileNotFound is returned when an agent profile with the given name does not exist.
var ErrAgentProfileNotFound = errors.New("agent profile not found")

// ErrAgentProfileNameConflict is returned when an agent profile with the given name already exists.
var ErrAgentProfileNameConflict = errors.New("agent profile name already exists")

// ExecutionSettings stores runtime-owned execution defaults for a profile.
type ExecutionSettings struct {
	// DefaultModel is the fully-qualified default model in provider/model form.
	DefaultModel string
}

// AgentProfile is a persisted general-purpose agent profile definition.
type AgentProfile struct {
	// Name is the immutable technical profile identifier.
	Name string

	// DisplayName is an optional human-friendly profile label.
	DisplayName string

	// Role describes the profile's high-level responsibility.
	Role string

	// Instructions stores profile-specific behavior guidance.
	Instructions string

	// ToolRefs lists tool references available to this profile.
	ToolRefs []string

	// ExecutionSettings stores Sonalmod-owned runtime settings.
	ExecutionSettings ExecutionSettings

	// CreatedAt is when the profile was first persisted.
	CreatedAt time.Time

	// UpdatedAt is when the profile was last updated.
	UpdatedAt time.Time
}

// CreateAgentProfileParams contains parameters required to create a profile.
type CreateAgentProfileParams struct {
	Name              string
	DisplayName       string
	Role              string
	Instructions      string
	ToolRefs          []string
	ExecutionSettings ExecutionSettings
}

// UpdateAgentProfileParams contains mutable parameters for profile updates.
type UpdateAgentProfileParams struct {
	DisplayName       string
	Role              string
	Instructions      string
	ToolRefs          []string
	ExecutionSettings ExecutionSettings
}

// AgentProfilesService manages persisted agent profiles.
//
//nolint:revive // service name is intentionally explicit and mirrored in public aliases.
type AgentProfilesService interface {
	// List returns all profiles sorted by CreatedAt ascending.
	List(ctx context.Context) ([]AgentProfile, error)

	// Get returns a profile by name.
	// Returns ErrAgentProfileNotFound when no profile exists with this name.
	Get(ctx context.Context, name string) (*AgentProfile, error)

	// Create creates a new profile.
	// Returns ErrAgentProfileNameConflict when a profile already exists with this name.
	Create(ctx context.Context, params CreateAgentProfileParams) (*AgentProfile, error)

	// Update modifies mutable fields of the profile identified by name.
	// Returns ErrAgentProfileNotFound when no profile exists with this name.
	Update(ctx context.Context, name string, params UpdateAgentProfileParams) (*AgentProfile, error)

	// Delete removes a profile by name.
	// Returns ErrAgentProfileNotFound when no profile exists with this name.
	Delete(ctx context.Context, name string) error

	// AutoMigrate applies required persistence migrations for the current backend.
	AutoMigrate() error
}

func normalizeCreateParams(params CreateAgentProfileParams) (CreateAgentProfileParams, error) {
	name := strings.TrimSpace(params.Name)
	if !agentProfileNamePattern.MatchString(name) {
		return CreateAgentProfileParams{}, fmt.Errorf(
			"invalid profile name %q: must match ^[a-z][a-z0-9-]*$",
			params.Name,
		)
	}

	normalizedToolRefs, err := normalizeToolRefs(params.ToolRefs)
	if err != nil {
		return CreateAgentProfileParams{}, err
	}

	execSettings, err := normalizeExecutionSettings(params.ExecutionSettings)
	if err != nil {
		return CreateAgentProfileParams{}, err
	}

	role, instructions, err := normalizeRoleAndInstructions(params.Role, params.Instructions)
	if err != nil {
		return CreateAgentProfileParams{}, err
	}

	params.Name = name
	params.DisplayName = strings.TrimSpace(params.DisplayName)
	params.Role = role
	params.Instructions = instructions
	params.ToolRefs = normalizedToolRefs
	params.ExecutionSettings = execSettings

	return params, nil
}

func applyProfileUpdate(existing AgentProfile, params UpdateAgentProfileParams) (AgentProfile, error) {
	normalizedToolRefs, err := normalizeToolRefs(params.ToolRefs)
	if err != nil {
		return AgentProfile{}, err
	}

	execSettings, err := normalizeExecutionSettings(params.ExecutionSettings)
	if err != nil {
		return AgentProfile{}, err
	}

	role, instructions, err := normalizeRoleAndInstructions(params.Role, params.Instructions)
	if err != nil {
		return AgentProfile{}, err
	}

	updated := existing
	updated.DisplayName = strings.TrimSpace(params.DisplayName)
	updated.Role = role
	updated.Instructions = instructions
	updated.ToolRefs = normalizedToolRefs
	updated.ExecutionSettings = execSettings

	return updated, nil
}

func normalizeRoleAndInstructions(role string, instructions string) (string, string, error) {
	normalizedRole := strings.TrimSpace(role)
	if normalizedRole == "" {
		return "", "", errors.New("role is required")
	}

	normalizedInstructions := strings.TrimSpace(instructions)
	if normalizedInstructions == "" {
		return "", "", errors.New("instructions are required")
	}

	return normalizedRole, normalizedInstructions, nil
}

func normalizeExecutionSettings(settings ExecutionSettings) (ExecutionSettings, error) {
	settings.DefaultModel = strings.TrimSpace(settings.DefaultModel)
	if settings.DefaultModel == "" {
		return ExecutionSettings{}, errors.New("execution_settings.default_model is required")
	}
	return settings, nil
}

func normalizeToolRefs(toolRefs []string) ([]string, error) {
	if toolRefs == nil {
		return []string{}, nil
	}

	normalized := make([]string, 0, len(toolRefs))
	for _, ref := range toolRefs {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			return nil, errors.New("tool_refs must not contain empty values")
		}
		if slices.Contains(normalized, trimmed) {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	return normalized, nil
}
