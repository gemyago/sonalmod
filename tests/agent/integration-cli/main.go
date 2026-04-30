package main

import (
	"fmt"
	"os"

	"github.com/gemyago/sonalmod/apps/sonalmod"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
)

type runCmdArgs struct {
	rootArgsVals *rootArgs
	params       cliParams
}

func buildEngine(rootArgsVals *rootArgs) (*sonalmod.Engine, error) {
	opts := []sonalmod.EngineOpt{
		sonalmod.WithEngineLogsFormatJSON(rootArgsVals.jsonLogs),
		sonalmod.WithEngineLogsOutputFile(rootArgsVals.logsFile),
		sonalmod.WithEngineDefaultLogLevel(rootArgsVals.logLevel),
	}
	if rootArgsVals.env != "" {
		opts = append(opts, sonalmod.WithEngineEnv(rootArgsVals.env))
	}
	return sonalmod.NewEngine(opts...)
}

func buildListModelsFunc(rootArgsVals *rootArgs) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error { // coverage-ignore
		engine, err := buildEngine(rootArgsVals)
		if err != nil {
			return fmt.Errorf("create engine: %w", err)
		}
		runner, err := engine.GetAgentRunner()
		if err != nil {
			return fmt.Errorf("create runner: %w", err)
		}
		return runListModels(cmd.Context(), runner.ModelsLocator(), os.Stdout)
	}
}

func buildRunFunc(args *runCmdArgs) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error { // coverage-ignore
		engine, err := buildEngine(args.rootArgsVals)
		if err != nil {
			return fmt.Errorf("create engine: %w", err)
		}

		toolsRegistry, err := engine.GetToolsRegistry()
		if err != nil {
			return fmt.Errorf("get tools registry: %w", err)
		}
		registerTestTools(toolsRegistry)

		runner, err := engine.GetAgentRunner()
		if err != nil {
			return fmt.Errorf("create runner: %w", err)
		}

		return runCLI(cmd.Context(), runner, args.params, os.Stdout)
	}
}

type rootArgs struct {
	logLevel string
	logsFile string
	jsonLogs bool
	env      string
}

func setupCommands() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "integration-cli",
		Short:        "Integration test CLI for agent testing",
		SilenceUsage: true,
	}
	var rootArgsVals rootArgs
	rootCmd.PersistentFlags().StringVarP(&rootArgsVals.logLevel, "log-level", "l", "", "Log level")
	rootCmd.PersistentFlags().StringVar(&rootArgsVals.logsFile, "logs-file", "integration-cli.log", "Log output file")
	rootCmd.PersistentFlags().BoolVar(&rootArgsVals.jsonLogs, "json-logs", true, "JSON log format")
	rootCmd.PersistentFlags().StringVarP(&rootArgsVals.env, "env", "e", "", "Environment name")

	cmdArgs := &runCmdArgs{
		rootArgsVals: &rootArgsVals,
	}
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the agent from the command line",
		RunE:  buildRunFunc(cmdArgs),
	}
	cmd.Flags().StringVarP(&cmdArgs.params.Prompt, "prompt", "p", "", "User prompt for the agent")
	cmd.Flags().StringVarP(&cmdArgs.params.SessionID, "session", "s", "", "Session ID to reuse")
	cmd.Flags().StringVar(&cmdArgs.params.Model, "model", "", "Fully qualified model name (e.g. provider/model)")
	lo.Must0(cmd.MarkFlagRequired("prompt"))
	lo.Must0(cmd.MarkFlagRequired("model"))
	rootCmd.AddCommand(cmd)

	listModelsCmd := &cobra.Command{
		Use:   "list-models",
		Short: "List configured models (fully qualified provider/model names)",
		RunE:  buildListModelsFunc(&rootArgsVals),
	}
	rootCmd.AddCommand(listModelsCmd)

	acpArgs := &acpCmdArgs{}
	acpCmd := &cobra.Command{
		Use:   "acp",
		Short: "Probe an ACP-compatible agent over stdio JSON-RPC",
		RunE:  buildACPFunc(acpArgs),
	}
	acpCmd.Flags().StringVar(
		&acpArgs.AgentCommand,
		"agent-command",
		"",
		"Agent executable to run (for example: opencode)",
	)
	acpCmd.Flags().StringArrayVar(
		&acpArgs.AgentArgs,
		"agent-arg",
		nil,
		"Argument to pass to the agent command (repeatable)",
	)
	acpCmd.Flags().StringVar(&acpArgs.CWD, "cwd", "", "Working directory for the ACP agent process")
	acpCmd.Flags().StringVarP(&acpArgs.Prompt, "prompt", "p", "", "Prompt to send via session/prompt")
	acpCmd.Flags().StringVar(
		&acpArgs.TranscriptPath,
		"transcript",
		"",
		"Optional JSONL transcript path for ACP envelopes",
	)
	acpCmd.Flags().StringVar(
		&acpArgs.LoadSessionID,
		"load-session",
		"",
		"Optional session ID to load instead of creating a new one",
	)
	acpCmd.Flags().DurationVar(
		&acpArgs.CancelAfter,
		"cancel-after",
		0,
		"Optional delay before sending session/cancel",
	)
	lo.Must0(acpCmd.MarkFlagRequired("agent-command"))
	lo.Must0(acpCmd.MarkFlagRequired("prompt"))
	rootCmd.AddCommand(acpCmd)

	return rootCmd
}

func main() { // coverage-ignore
	rootCmd := setupCommands()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
