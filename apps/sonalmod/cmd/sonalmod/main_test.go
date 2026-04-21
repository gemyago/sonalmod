package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.log")
}

// chdirModuleRoot sets cwd to apps/sonalmod so embedded config paths (e.g. dataDir: data) match pre-cmd layout.
func chdirModuleRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	moduleRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	t.Chdir(moduleRoot)
}

func TestMain(t *testing.T) {
	chdirModuleRoot(t)
	fake := faker.New()
	t.Run("start", func(t *testing.T) {
		t.Run("should initialize app", func(t *testing.T) {
			rootCmd := setupCommands()
			rootCmd.SetArgs([]string{"start", "-e", "test", "--noop", "--logs-file", testLogFile(t)})
			require.NoError(t, rootCmd.Execute())
		})
		t.Run("should fail if bad log level", func(t *testing.T) {
			rootCmd := setupCommands()
			rootCmd.SilenceErrors = true
			rootCmd.SilenceUsage = true
			rootCmd.SetArgs(
				[]string{"start", "-e", "test", "--noop", "-l", fake.Lorem().Word(), "--logs-file", testLogFile(t)},
			)
			assert.Error(t, rootCmd.Execute())
		})
		t.Run("--ui-location flag", func(t *testing.T) {
			t.Run("should accept --ui-location flag and complete setup without error", func(t *testing.T) {
				wantUILocation := t.TempDir()
				rootCmd := setupCommands()
				rootCmd.SetArgs([]string{
					"start", "-e", "test", "--noop",
					"--ui-location", wantUILocation,
					"--logs-file", testLogFile(t),
				})
				require.NoError(t, rootCmd.Execute())
			})

			t.Run("should expose ui-location flag on start command", func(t *testing.T) {
				rootCmd := setupCommands()
				startCmd := findStartCmd(t, rootCmd)
				assert.NotNil(t, startCmd.Flags().Lookup("ui-location"))
			})

			t.Run("should default to empty string when --ui-location is not provided", func(t *testing.T) {
				rootCmd := setupCommands()
				startCmd := findStartCmd(t, rootCmd)
				rootCmd.SetArgs([]string{"start", "-e", "test", "--noop", "--logs-file", testLogFile(t)})
				require.NoError(t, rootCmd.Execute())
				uiLoc, err := startCmd.Flags().GetString("ui-location")
				require.NoError(t, err)
				assert.Empty(t, uiLoc)
			})
		})
	})
}

func findStartCmd(t *testing.T, rootCmd *cobra.Command) *cobra.Command {
	t.Helper()
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "start" {
			return cmd
		}
	}
	t.Fatal("start command not found")
	return nil
}

func TestDevFlow(t *testing.T) {
	chdirModuleRoot(t)
	t.Run("default startup is API-only and requires no UI build artifacts", func(t *testing.T) {
		t.Run("start without --ui-location completes setup without error", func(t *testing.T) {
			rootCmd := setupCommands()
			rootCmd.SetArgs([]string{"start", "-e", "test", "--noop", "--logs-file", testLogFile(t)})
			require.NoError(t, rootCmd.Execute())
		})

		t.Run("start command ui-location flag defaults to empty string", func(t *testing.T) {
			rootCmd := setupCommands()
			startCmd := findStartCmd(t, rootCmd)
			rootCmd.SetArgs([]string{"start", "-e", "test", "--noop", "--logs-file", testLogFile(t)})
			require.NoError(t, rootCmd.Execute())
			uiLoc, err := startCmd.Flags().GetString("ui-location")
			require.NoError(t, err)
			// ui-location must default to empty so server starts in API-only mode.
			assert.Empty(t, uiLoc)
		})
	})
}
