package main

import (
	"fmt"

	"github.com/gemyago/sonalmod/apps/sonalmod"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal"
	"github.com/spf13/cobra"
	"go.uber.org/dig"
)

func newEngineFromRoot(root *cobra.Command, container *dig.Container) (*sonalmod.Engine, error) {
	fs := root.PersistentFlags()
	jsonLogs, err := fs.GetBool("json-logs")
	if err != nil {
		return nil, fmt.Errorf("json-logs: %w", err)
	}
	logsFile, err := fs.GetString("logs-file")
	if err != nil {
		return nil, fmt.Errorf("logs-file: %w", err)
	}
	defaultLogLevel, err := fs.GetString("log-level")
	if err != nil {
		return nil, fmt.Errorf("log-level: %w", err)
	}
	env, err := fs.GetString("env")
	if err != nil {
		return nil, fmt.Errorf("env: %w", err)
	}

	opts := []sonalmod.EngineOpt{
		sonalmod.WithEngineLogsFormatJSON(jsonLogs),
		sonalmod.WithEngineLogsOutputFile(logsFile),
		sonalmod.WithEngineDefaultLogLevel(defaultLogLevel),
		internal.WithEngineContainer(container),
	}
	if env != "" {
		opts = append(opts, sonalmod.WithEngineEnv(env))
	}
	return sonalmod.NewEngine(opts...)
}
