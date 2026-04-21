package sonalmod

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/server"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/config"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/system/lifecycle"
	"github.com/gemyago/sonalmod/runtime/agent"
	"github.com/spf13/viper"
	"go.uber.org/dig"
)

type Engine struct {
	container *dig.Container
	cfg       *viper.Viper
}

func NewEngine(opts ...EngineOpt) (*Engine, error) {
	rootCtx := context.Background()

	o := &internal.EngineCfg{
		LogsFormatJSON: false,
		LogsOutputFile: "",
		Container:      dig.New(),
		Config:         config.New(),
	}
	o.Apply(opts...)

	cfg := o.Config
	container := o.Container

	if o.LogsFormatJSON {
		cfg.Set("jsonLogs", true)
	}
	if o.LogsOutputFile != "" {
		cfg.Set("logs-file", o.LogsOutputFile)
	}
	if o.DefaultLogLevel != nil {
		cfg.Set("defaultLogLevel", *o.DefaultLogLevel)
	}
	if o.Env != "" {
		cfg.Set("env", o.Env)
	}

	if err := internal.Setup(rootCtx, cfg, container); err != nil {
		return nil, fmt.Errorf("failed to setup engine: %w", err)
	}

	return &Engine{container: container, cfg: cfg}, nil
}

func (e *Engine) GetToolsRegistry() (*agent.ToolsRegistry, error) {
	var reg *agent.ToolsRegistry
	if err := e.container.Invoke(func(r *agent.ToolsRegistry) {
		reg = r
	}); err != nil {
		return nil, fmt.Errorf("failed resolve tools registry: %w", err)
	}
	return reg, nil
}

func (e *Engine) GetAgentRunner() (*agent.Runner, error) {
	var runner *agent.Runner
	if err := e.container.Invoke(func(r *internal.Runtime) {
		runner = r.Runner
	}); err != nil {
		return nil, fmt.Errorf("failed resolve agent runner: %w", err)
	}
	return runner, nil
}

// StartHTTPServer starts the HTTP server.
// Note: this method is blocking until the HTTP server is stopped.
// Note: this method is supposed to be called only once.
// It will also handle system signals and shutdown the HTTP server gracefully.
func (e *Engine) StartHTTPServer(ctx context.Context, opts ...EngineStartServerOpt) error {
	o := &engineStartServerCfg{
		noop: false,
	}
	for _, opt := range opts {
		opt.apply(o)
	}

	if o.uiLocation != "" {
		e.cfg.Set("httpServer.uiLocation", o.uiLocation)
	}

	if err := errors.Join(
		server.Register(e.container),
		http.Register(e.container),
	); err != nil {
		return fmt.Errorf("failed to register HTTP server: %w", err)
	}

	type startServerDeps struct {
		dig.In

		StartupGroupFactory lifecycle.StartupGroupFactory
		RootLogger          *slog.Logger

		HTTPServer *server.HTTPServer
	}

	return e.container.Invoke(func(deps startServerDeps) error {
		rootLogger := deps.RootLogger
		httpServer := deps.HTTPServer

		startupGroup := deps.StartupGroupFactory.NewGroup()
		startupGroup.Add(func(groupCtx context.Context) error {
			if o.noop {
				rootLogger.InfoContext(groupCtx, "NOOP: Starting http server")
				return nil
			}
			return httpServer.Start(groupCtx)
		})

		return startupGroup.Start(ctx)
	})
}
