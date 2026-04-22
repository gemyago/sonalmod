package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gemyago/sonalmod/runtime/agent"
	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/gemyago/sonalmod/runtime/internal/agentapi"
)

// HandlerArgs holds the required dependencies for NewHandler.
type HandlerArgs struct {
	// Runner is typically a [*agent.Runner].
	Runner agent.AgentRunner

	// ProvidersConfigService is required.
	ProvidersConfigService agent.ProvidersConfigService
	// AgentProfilesService is required.
	AgentProfilesService agent.AgentProfilesService

	// ModelsLister is optional. When provided, enables the GET /models endpoint.
	// Typically set to the runner's models locator when dynamic providers are used.
	// Use [agent.Runner.ModelsLocator] when dynamic providers are configured.
	ModelsLister agent.ModelsLister
}

type handlerOpts struct {
	Logger *slog.Logger
}

type HandlerOpt func(*handlerOpts)

func WithLogger(logger *slog.Logger) HandlerOpt {
	return func(opts *handlerOpts) {
		opts.Logger = logger
	}
}

func NewHandler(args HandlerArgs, opts ...HandlerOpt) (http.Handler, error) {
	if args.Runner == nil {
		return nil, errors.New("runner is required")
	}
	if args.ProvidersConfigService == nil {
		return nil, errors.New("providers config service is required")
	}
	if args.AgentProfilesService == nil {
		return nil, errors.New("agent profiles service is required")
	}

	hOpts := &handlerOpts{
		Logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(hOpts)
	}

	agentRunner := rt.NewBackgroundRunner(rt.BackgroundRunnerDeps{
		Runner: args.Runner,
		Logger: hOpts.Logger,
	})
	server := agentapi.NewAgentAPIServer(agentapi.ServerParams{
		Runner:                 agentRunner,
		Logger:                 hOpts.Logger.WithGroup("sonalmod.runtime.httpapi.handler"),
		IDGen:                  agentapi.NewDefaultIDGen(),
		RequestMapper:          agentapi.NewAgentAPIRequestMapper(),
		SSEWriter:              agentapi.NewAgentAPISSEWriter(agentapi.NewAgentAPIStreamEventMapper()),
		ProvidersConfigService: args.ProvidersConfigService,
		AgentProfilesService:   args.AgentProfilesService,
		ModelsLister:           args.ModelsLister,
	})
	return agentapi.HandlerFromMux(server, http.NewServeMux()), nil
}
