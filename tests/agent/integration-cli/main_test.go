package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupCommands(t *testing.T) {
	t.Run("root command has expected persistent flags", func(t *testing.T) {
		root := setupCommands()
		fs := root.PersistentFlags()

		logLevel, err := fs.GetString("log-level")
		require.NoError(t, err)
		assert.Empty(t, logLevel, "log-level default should be empty")

		logsFile, err := fs.GetString("logs-file")
		require.NoError(t, err)
		assert.Equal(t, "integration-cli.log", logsFile, "logs-file default")

		jsonLogs, err := fs.GetBool("json-logs")
		require.NoError(t, err)
		assert.True(t, jsonLogs, "json-logs default should be true")

		env, err := fs.GetString("env")
		require.NoError(t, err)
		assert.Empty(t, env, "env default should be empty")
	})

	t.Run("list-models subcommand is registered", func(t *testing.T) {
		root := setupCommands()
		listCmd, _, err := root.Find([]string{"list-models"})
		require.NoError(t, err)
		require.NotNil(t, listCmd)
		assert.Equal(t, "list-models", listCmd.Use)
	})

	t.Run("run subcommand has prompt and session flags", func(t *testing.T) {
		root := setupCommands()

		runCmd, _, err := root.Find([]string{"run"})
		require.NoError(t, err)
		require.NotNil(t, runCmd)
		assert.Equal(t, "run", runCmd.Use)

		prompt, err := runCmd.Flags().GetString("prompt")
		require.NoError(t, err)
		assert.Empty(t, prompt, "prompt default should be empty")

		session, err := runCmd.Flags().GetString("session")
		require.NoError(t, err)
		assert.Empty(t, session, "session default should be empty")
	})

	t.Run("acp subcommand is registered with required flags", func(t *testing.T) {
		root := setupCommands()

		acpCmd, _, err := root.Find([]string{"acp"})
		require.NoError(t, err)
		require.NotNil(t, acpCmd)
		assert.Equal(t, "acp", acpCmd.Use)

		annotation := acpCmd.Flags().Lookup("agent-command").Annotations
		_, required := annotation["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, required, "--agent-command should be marked as required")

		annotation = acpCmd.Flags().Lookup("prompt").Annotations
		_, required = annotation["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, required, "--prompt should be marked as required")
	})

	t.Run("run subcommand prompt flag is required", func(t *testing.T) {
		root := setupCommands()
		runCmd, _, err := root.Find([]string{"run"})
		require.NoError(t, err)
		require.NotNil(t, runCmd)

		annotation := runCmd.Flags().Lookup("prompt").Annotations
		_, required := annotation["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, required, "--prompt should be marked as required")
	})

	t.Run("persistent flags can be overridden via args", func(t *testing.T) {
		root := setupCommands()
		root.SetArgs([]string{"--log-level", "debug", "--json-logs=false", "--env", "staging"})
		// Execute just the root (no subcommand), expect error about missing subcommand but flags parsed.
		_ = root.Execute()

		fs := root.PersistentFlags()
		logLevel, err := fs.GetString("log-level")
		require.NoError(t, err)
		assert.Equal(t, "debug", logLevel)

		jsonLogs, err := fs.GetBool("json-logs")
		require.NoError(t, err)
		assert.False(t, jsonLogs)

		env, err := fs.GetString("env")
		require.NoError(t, err)
		assert.Equal(t, "staging", env)
	})
}
