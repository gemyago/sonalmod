package agent

import (
	"context"
	"log/slog"

	"github.com/gemyago/sonalmod/runtime/internal"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
)

// ProvidersConfigService is the provider configuration management contract.
type ProvidersConfigService = lp.ProvidersConfigService

// ModelConfig represents a single model available through a provider.
type ModelConfig = lp.ModelConfig

// CreateProviderConfigParams holds the parameters for creating a new provider config.
type CreateProviderConfigParams = lp.CreateProviderConfigParams

// ModelInfo describes a single model available through a provider.
type ModelInfo = internal.ModelInfo

// ModelsLister lists available models across all configured providers.
// [*Runner.ModelsLocator] returns a value satisfying this interface for runners
// constructed with [NewRunner] and a non-nil [RunnerArgs.ProvidersConfigService].
type ModelsLister interface {
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// NewFileProvidersConfigService creates a file-based [ProvidersConfigService] that stores
// provider configurations as YAML files under {baseDir}/providers/ (canonical {name}.yaml;
// optional same-named {name}.yml for discovery and reads).
func NewFileProvidersConfigService( //nolint:ireturn
	baseDir string,
	logger *slog.Logger,
) (ProvidersConfigService, error) {
	return lp.NewFileProvidersConfigService(baseDir, logger)
}

// NewDatabaseProvidersConfigService creates a database-backed [ProvidersConfigService] that stores
// provider configurations in a relational database identified by the given DSN.
// tablePrefix sets the prefix for persisted SQL table names; empty means no prefix.
func NewDatabaseProvidersConfigService( //nolint:ireturn
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (ProvidersConfigService, error) {
	return lp.NewDatabaseProvidersConfigService(dsn, logger, tablePrefix)
}
