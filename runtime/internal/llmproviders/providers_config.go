package llmproviders

import (
	"context"
	"errors"
	"regexp"
	"time"
)

var providerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ProviderTypeOpenAICompatible is the provider type for OpenAI-compatible endpoints.
const ProviderTypeOpenAICompatible = "openai-compatible"

// ErrProviderConfigNotFound is returned when a provider config with the given name does not exist.
var ErrProviderConfigNotFound = errors.New("provider config not found")

// ErrProviderConfigNameConflict is returned when a provider config with the given name already exists.
var ErrProviderConfigNameConflict = errors.New("provider config name already exists")

// ModelConfig represents a single model available through a provider.
type ModelConfig struct {
	// Name is the technical model identifier (e.g., "gpt-4.1").
	Name string

	// DisplayName is an optional human-friendly label (e.g., "GPT 4.1").
	DisplayName string

	// Summarization designates this model for summarization tasks (e.g. session titles).
	// If multiple models have this set, the first one found when listing providers is used.
	Summarization bool
}

// ProviderConfig represents a configured LLM provider endpoint.
type ProviderConfig struct {
	// Name is the unique technical identifier used as the model-name prefix (e.g., "openai").
	// Immutable after creation.
	Name string

	// Type is the provider protocol type (e.g., "openai-compatible").
	// Immutable after creation.
	Type string

	// DisplayName is an optional human-friendly label.
	DisplayName string

	// BaseURL is the base URL of the provider API endpoint.
	BaseURL string

	// APIKey is the API key for authentication. Never returned in full via API responses.
	APIKey string `json:"-"`

	// Models is the list of models available through this provider.
	Models []ModelConfig

	// CreatedAt is the time the provider config was created.
	CreatedAt time.Time

	// UpdatedAt is the time the provider config was last updated.
	UpdatedAt time.Time
}

// CreateProviderConfigParams holds the parameters for creating a new provider config.
type CreateProviderConfigParams struct {
	Name        string
	Type        string
	DisplayName string
	BaseURL     string
	APIKey      string `json:"-"`
	Models      []ModelConfig
}

// UpdateProviderConfigParams holds the parameters for updating an existing provider config.
// APIKey is optional — when empty, the current value is preserved.
type UpdateProviderConfigParams struct {
	DisplayName string
	BaseURL     string
	APIKey      string `json:"-"`
	Models      []ModelConfig
}

// ProvidersConfigService manages LLM provider configurations.
type ProvidersConfigService interface {
	// List returns all provider configs sorted by CreatedAt ascending.
	List(ctx context.Context) ([]ProviderConfig, error)

	// Get returns the provider config with the given name.
	// Returns ErrProviderConfigNotFound if no config with that name exists.
	Get(ctx context.Context, name string) (*ProviderConfig, error)

	// Create creates a new provider config.
	// Returns ErrProviderConfigNameConflict if a config with the same name already exists.
	Create(ctx context.Context, params CreateProviderConfigParams) (*ProviderConfig, error)

	// Update updates the provider config with the given name.
	// Returns ErrProviderConfigNotFound if no config with that name exists.
	Update(ctx context.Context, name string, params UpdateProviderConfigParams) (*ProviderConfig, error)

	// Delete removes the provider config with the given name.
	// Returns ErrProviderConfigNotFound if no config with that name exists.
	Delete(ctx context.Context, name string) error
}
