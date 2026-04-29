package internal

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai"
	lp "github.com/gemyago/sonalmod/runtime/internal/llmproviders"
	"github.com/gemyago/sonalmod/runtime/internal/summarize"
	"google.golang.org/adk/model"
)

// ModelInfo describes a single model available through a provider.
type ModelInfo struct {
	// Provider is the provider name (prefix in fully-qualified model names).
	Provider string

	// Name is the technical model identifier (e.g., "gpt-4.1").
	Name string

	// DisplayName is an optional human-friendly label.
	DisplayName string
}

// genkitInitFunc is the function signature for initializing a genkit instance.
// Matches genkit.Init for production; injectable for tests.
type genkitInitFuncType func(ctx context.Context, cfg lp.ProviderConfig) (*genkit.Genkit, error)

// cachedGenkitInstance holds a genkit instance together with the provider's UpdatedAt stamp.
type cachedGenkitInstance struct {
	g         *genkit.Genkit
	updatedAt time.Time
}

// ModelsLocatorParams holds dependencies for constructing a ModelsLocator.
type ModelsLocatorParams struct {
	ProvidersSvc      lp.ProvidersConfigService
	Logger            *slog.Logger
	GenkitInitFunc    genkitInitFuncType   // injectable; defaults to defaultGenkitInit
	ToolStubRegistrar func(*genkit.Genkit) // required; called on each new genkit instance
}

// ModelsLocator resolves model.LLM adapters from fully-qualified model names and
// caches genkit instances per provider. Cache invalidation is based on UpdatedAt.
type ModelsLocator struct {
	mu                sync.Mutex
	providersSvc      lp.ProvidersConfigService
	cache             map[string]*cachedGenkitInstance
	genkitInitFunc    genkitInitFuncType
	toolStubRegistrar func(*genkit.Genkit)
	logger            *slog.Logger
}

// NewModelsLocator constructs a ModelsLocator. ToolStubRegistrar must be non-nil.
func NewModelsLocator(params ModelsLocatorParams) *ModelsLocator {
	initFn := params.GenkitInitFunc
	if initFn == nil {
		initFn = defaultGenkitInit
	}
	return &ModelsLocator{
		providersSvc:      params.ProvidersSvc,
		cache:             make(map[string]*cachedGenkitInstance),
		genkitInitFunc:    initFn,
		toolStubRegistrar: params.ToolStubRegistrar,
		logger:            params.Logger,
	}
}

// parseModelName splits a fully-qualified model name "provider/model" into its two parts.
func parseModelName(fqModelName string) (string, string, error) {
	provider, modelName, ok := strings.Cut(fqModelName, "/")
	if !ok {
		return "", "", fmt.Errorf("invalid model name %q: expected \"provider/model\" format", fqModelName)
	}
	return provider, modelName, nil
}

// ResolveModel returns a model.LLM for the given fully-qualified model name.
// It parses provider/model, looks up the provider config, manages a per-provider
// genkit cache (invalidated by UpdatedAt), and creates a new genkit instance when needed.
//
//nolint:ireturn // ADK expects the model.LLM interface return shape here.
func (l *ModelsLocator) ResolveModel(
	ctx context.Context,
	fqModelName string,
) (model.LLM, error) {
	providerName, _, err := parseModelName(fqModelName)
	if err != nil {
		return nil, err
	}

	cfg, err := l.providersSvc.Get(ctx, providerName)
	if err != nil {
		return nil, fmt.Errorf("get provider %q: %w", providerName, err)
	}

	g, err := l.resolveGenkitInstance(ctx, providerName, *cfg)
	if err != nil {
		return nil, err
	}

	factory := NewGenkitLLMAdapterFactory(GenkitLLMAdapterDeps{
		Genkit:     g,
		RootLogger: l.logger,
	})

	// Genkit requires fully-qualified model name
	return factory(fqModelName), nil
}

// resolveGenkitInstance returns a cached (or newly created) genkit instance for the provider.
// The mutex ensures only one goroutine initializes for the same provider at a time.
func (l *ModelsLocator) resolveGenkitInstance(
	ctx context.Context, providerName string, cfg lp.ProviderConfig,
) (*genkit.Genkit, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cached, ok := l.cache[providerName]; ok && cached.updatedAt.Equal(cfg.UpdatedAt) {
		return cached.g, nil
	}

	l.logger.DebugContext(ctx, "initializing genkit instance for provider",
		slog.String("provider", providerName),
	)

	g, err := l.genkitInitFunc(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("init genkit for provider %q: %w", providerName, err)
	}

	l.toolStubRegistrar(g)

	l.cache[providerName] = &cachedGenkitInstance{
		g:         g,
		updatedAt: cfg.UpdatedAt,
	}

	return g, nil
}

// ListModels returns all configured models across all providers.
func (l *ModelsLocator) ListModels(ctx context.Context) ([]ModelInfo, error) {
	providers, err := l.providersSvc.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}

	var models []ModelInfo
	for _, p := range providers {
		for _, m := range p.Models {
			models = append(models, ModelInfo{
				Provider:    p.Name,
				Name:        m.Name,
				DisplayName: m.DisplayName,
			})
		}
	}

	if models == nil {
		return []ModelInfo{}, nil
	}
	return models, nil
}

// defaultGenkitInit is the production genkit.Init implementation.
func defaultGenkitInit(ctx context.Context, cfg lp.ProviderConfig) (*genkit.Genkit, error) {
	plugin := openAICompatiblePlugin(cfg)
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))
	return g, nil
}

// openAICompatiblePlugin creates the genkit plugin for an OpenAI-compatible provider.
func openAICompatiblePlugin(cfg lp.ProviderConfig) *oai.OpenAICompatible {
	return &oai.OpenAICompatible{
		Provider: cfg.Name,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	}
}

var _ summarize.ModelResolver = (*ModelsLocator)(nil)
