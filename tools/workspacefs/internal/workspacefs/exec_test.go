package workspacefs

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecCommand(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	makeService := func(t *testing.T, opts ...ServiceOption) (*Service, string) {
		t.Helper()
		dir := t.TempDir()
		svc, err := NewService([]WorkspaceMount{
			{Identifier: "test", Description: "test workspace", Path: dir},
		}, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc, dir
	}

	defaultCfg := func() ExecConfig {
		return ExecConfig{
			MaxOutputBytes:    1024 * 1024,
			DefaultTimeout:    10 * time.Second,
			MaxConcurrentJobs: 10,
		}
	}

	t.Run("exec_disabled_returns_error", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t) // exec not enabled
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exec")
	})

	t.Run("workspace_required", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Command: "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace")
	})

	t.Run("unknown_workspace_returns_error", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: fake.UUID().V4(),
			Command:   "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace")
	})

	t.Run("success_path", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		word := fake.Lorem().Word()
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   echoCmd(word),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, word)
		assert.False(t, resp.TimedOut)
		assert.False(t, resp.Truncated)
	})

	t.Run("non_zero_exit_code", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   exitCmd(2),
		})
		require.NoError(t, err)
		assert.Equal(t, 2, resp.ExitCode)
		assert.False(t, resp.TimedOut)
	})

	t.Run("timeout_behavior", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.DefaultTimeout = 100 * time.Millisecond
		svc, _ := makeService(t, WithExecEnabled(cfg))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   sleepCmd(5),
		})
		require.NoError(t, err)
		assert.True(t, resp.TimedOut)
		assert.NotEqual(t, 0, resp.ExitCode)
	})

	t.Run("output_truncation", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.MaxOutputBytes = 10
		svc, _ := makeService(t, WithExecEnabled(cfg))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   echoCmd(strings.Repeat("x", 100)),
		})
		require.NoError(t, err)
		assert.True(t, resp.Truncated)
		assert.LessOrEqual(t, len(resp.Stdout), int(cfg.MaxOutputBytes))
	})

	t.Run("working_directory_relative", func(t *testing.T) {
		t.Parallel()
		svc, dir := makeService(t, WithExecEnabled(defaultCfg()))
		subdir := fake.UUID().V4()
		require.NoError(t, os.Mkdir(filepath.Join(dir, subdir), 0o755))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:        "test",
			WorkingDirectory: subdir,
			Command:          pwdCmd(),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, subdir)
	})

	t.Run("working_directory_absolute_rejected", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:        "test",
			WorkingDirectory: "/etc",
			Command:          "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "working")
	})

	t.Run("working_directory_escape_rejected", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:        "test",
			WorkingDirectory: "../outside",
			Command:          "echo hello",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "working")
	})

	t.Run("stderr_captured", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		word := fake.Lorem().Word()
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   stderrCmd(word),
		})
		require.NoError(t, err)
		assert.Contains(t, resp.Stderr, word)
	})

	t.Run("no_host_path_in_error", func(t *testing.T) {
		t.Parallel()
		svc, dir := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:        "test",
			WorkingDirectory: "/absolute/path",
			Command:          "echo hello",
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), dir)
	})

	t.Run("empty_command_returns_error", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   "   ",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "command")
	})

	t.Run("background_returns_job_id", func(t *testing.T) {
		t.Parallel()
		svc, _ := makeService(t, WithExecEnabled(defaultCfg()))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(10),
			Background: true,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.JobID)
		assert.True(t, resp.Running)
		assert.Equal(t, 0, resp.ExitCode)
	})

	t.Run("stderr_truncated_when_output_exceeds_limit", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.MaxOutputBytes = 5
		svc, _ := makeService(t, WithExecEnabled(cfg))
		word := fake.Lorem().Word()
		// Write to stderr to test stderr truncation
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   stderrCmd(strings.Repeat(word, 10)),
		})
		require.NoError(t, err)
		assert.True(t, resp.Truncated)
		assert.LessOrEqual(t, len(resp.Stderr), int(cfg.MaxOutputBytes))
	})
}

func TestExecBackground(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	makeService := func(t *testing.T, opts ...ServiceOption) *Service {
		t.Helper()
		dir := t.TempDir()
		svc, err := NewService([]WorkspaceMount{
			{Identifier: "test", Description: "test workspace", Path: dir},
		}, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc
	}

	defaultCfg := func() ExecConfig {
		return ExecConfig{
			MaxOutputBytes:    1024 * 1024,
			DefaultTimeout:    30 * time.Second,
			MaxConcurrentJobs: 10,
		}
	}

	t.Run("background_start_returns_job_id", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(10),
			Background: true,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.JobID)
		assert.True(t, resp.Running)
	})

	t.Run("poll_running_job_returns_running_true", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))

		start, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(10),
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		poll, err := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
			Workspace: "test",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.True(t, poll.Running)
	})

	t.Run("poll_completed_job_returns_running_false_with_exit_code", func(t *testing.T) {
		t.Parallel()
		word := fake.Lorem().Word()
		svc := makeService(t, WithExecEnabled(defaultCfg()))

		start, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    echoCmd(word),
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		// Wait for the job to complete.
		require.Eventually(t, func() bool {
			poll, pollErr := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
				Workspace: "test",
				JobID:     start.JobID,
			})
			return pollErr == nil && !poll.Running
		}, 5*time.Second, 50*time.Millisecond)

		poll, err := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
			Workspace: "test",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.False(t, poll.Running)
		assert.Equal(t, 0, poll.ExitCode)
		assert.Contains(t, poll.Stdout, word)
		assert.NotEmpty(t, poll.Elapsed)
	})

	t.Run("kill_active_job", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))

		start, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(30),
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		kill, err := svc.ExecKillJob(t.Context(), ExecKillJobRequest{
			Workspace: "test",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.True(t, kill.Killed)

		// After kill the job should be completed (not running).
		require.Eventually(t, func() bool {
			poll, pollErr := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
				Workspace: "test",
				JobID:     start.JobID,
			})
			return pollErr == nil && !poll.Running
		}, 3*time.Second, 50*time.Millisecond)
	})

	t.Run("kill_unknown_job_returns_not_killed", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))
		kill, err := svc.ExecKillJob(t.Context(), ExecKillJobRequest{
			Workspace: "test",
			JobID:     fake.UUID().V4(),
		})
		require.NoError(t, err)
		assert.False(t, kill.Killed)
	})

	t.Run("poll_unknown_job_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
			Workspace: "test",
			JobID:     fake.UUID().V4(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "job")
	})

	t.Run("max_concurrent_jobs_enforced", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.MaxConcurrentJobs = 2
		svc := makeService(t, WithExecEnabled(cfg))

		for range cfg.MaxConcurrentJobs {
			resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
				Workspace:  "test",
				Command:    sleepCmd(30),
				Background: true,
			})
			require.NoError(t, err)
			require.NotEmpty(t, resp.JobID)
		}

		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(30),
			Background: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "concurrent")
	})

	t.Run("job_output_exec_disabled_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t) // exec not enabled
		_, err := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
			Workspace: "test",
			JobID:     fake.UUID().V4(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exec")
	})

	t.Run("kill_job_exec_disabled_returns_error", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t) // exec not enabled
		_, err := svc.ExecKillJob(t.Context(), ExecKillJobRequest{
			Workspace: "test",
			JobID:     fake.UUID().V4(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exec")
	})

	t.Run("kill_finished_job_returns_not_killed", func(t *testing.T) {
		t.Parallel()
		svc := makeService(t, WithExecEnabled(defaultCfg()))
		word := fake.Lorem().Word()

		start, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    echoCmd(word),
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		// Wait for the job to finish.
		require.Eventually(t, func() bool {
			poll, pollErr := svc.ExecJobOutput(t.Context(), ExecJobOutputRequest{
				Workspace: "test",
				JobID:     start.JobID,
			})
			return pollErr == nil && !poll.Running
		}, 5*time.Second, 50*time.Millisecond)

		kill, err := svc.ExecKillJob(t.Context(), ExecKillJobRequest{
			Workspace: "test",
			JobID:     start.JobID,
		})
		require.NoError(t, err)
		assert.False(t, kill.Killed)
	})
}

func TestServiceClose(t *testing.T) {
	t.Parallel()

	t.Run("close_cancels_active_background_jobs", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		svc, err := NewService([]WorkspaceMount{
			{Identifier: "test", Description: "test workspace", Path: dir},
		}, WithExecEnabled(ExecConfig{
			MaxOutputBytes:    1024 * 1024,
			DefaultTimeout:    30 * time.Second,
			MaxConcurrentJobs: 10,
		}))
		require.NoError(t, err)

		start, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    sleepCmd(60),
			Background: true,
		})
		require.NoError(t, err)
		require.NotEmpty(t, start.JobID)

		// Close should terminate active jobs.
		require.NoError(t, svc.Close())
	})
}

func TestExecPolicy(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	makeService := func(t *testing.T, opts ...ServiceOption) (*Service, string) {
		t.Helper()
		dir := t.TempDir()
		svc, err := NewService([]WorkspaceMount{
			{Identifier: "test", Description: "test workspace", Path: dir},
		}, opts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Close() })
		return svc, dir
	}

	defaultCfg := func() ExecConfig {
		return ExecConfig{
			MaxOutputBytes:    1024 * 1024,
			DefaultTimeout:    10 * time.Second,
			MaxConcurrentJobs: 10,
		}
	}

	t.Run("blocked_command_returns_error", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.BlockedCommands = []string{"curl", "wget"}
		svc, _ := makeService(t, WithExecEnabled(cfg))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   "curl https://example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked")
	})

	t.Run("blocked_command_with_leading_spaces_rejected", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.BlockedCommands = []string{"wget"}
		svc, _ := makeService(t, WithExecEnabled(cfg))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   "  wget https://example.com",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked")
	})

	t.Run("non_blocked_command_succeeds", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.BlockedCommands = []string{"curl", "wget"}
		svc, _ := makeService(t, WithExecEnabled(cfg))
		word := fake.Lorem().Word()
		resp, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   echoCmd(word),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, resp.Stdout, word)
	})

	t.Run("blocked_command_error_does_not_leak_host_path", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.BlockedCommands = []string{"curl"}
		svc, dir := makeService(t, WithExecEnabled(cfg))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: "test",
			Command:   "curl https://example.com",
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), dir)
	})

	t.Run("default_blocked_commands_block_network_exfiltration", func(t *testing.T) {
		t.Parallel()
		// When BlockedCommands is nil, defaults should block high-risk commands.
		cfg := defaultCfg()
		// cfg.BlockedCommands remains nil — should use built-in defaults.
		svc, _ := makeService(t, WithExecEnabled(cfg))
		for _, cmd := range []string{"curl https://evil.example", "wget http://evil.example", "nc -l 9999"} {
			t.Run(cmd, func(t *testing.T) {
				t.Parallel()
				_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
					Workspace: "test",
					Command:   cmd,
				})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "blocked")
			})
		}
	})

	t.Run("no_host_path_leaked_in_unknown_workspace_error", func(t *testing.T) {
		t.Parallel()
		svc, dir := makeService(t, WithExecEnabled(defaultCfg()))
		unknownID := fake.UUID().V4()
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace: unknownID,
			Command:   echoCmd("hello"),
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), dir)
	})

	t.Run("no_host_path_leaked_in_working_dir_error", func(t *testing.T) {
		t.Parallel()
		svc, dir := makeService(t, WithExecEnabled(defaultCfg()))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:        "test",
			WorkingDirectory: "../escape",
			Command:          echoCmd("hello"),
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), dir)
	})

	t.Run("blocked_command_background_also_blocked", func(t *testing.T) {
		t.Parallel()
		cfg := defaultCfg()
		cfg.BlockedCommands = []string{"curl"}
		svc, _ := makeService(t, WithExecEnabled(cfg))
		_, err := svc.ExecCommand(t.Context(), ExecCommandRequest{
			Workspace:  "test",
			Command:    "curl https://example.com",
			Background: true,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "blocked")
	})
}

// echoCmd returns a platform-appropriate echo command.
func echoCmd(msg string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("cmd /c echo %s", msg)
	}
	return fmt.Sprintf("echo %s", msg)
}

// exitCmd returns a command that exits with the given code.
func exitCmd(code int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("cmd /c exit %d", code)
	}
	return fmt.Sprintf("sh -c 'exit %d'", code)
}

// sleepCmd returns a command that sleeps for the given seconds.
func sleepCmd(secs int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("cmd /c timeout /t %d /nobreak", secs)
	}
	return fmt.Sprintf("sleep %d", secs)
}

// pwdCmd returns a command that prints the current directory.
func pwdCmd() string {
	if runtime.GOOS == "windows" {
		return "cmd /c cd"
	}
	return "pwd"
}

// stderrCmd returns a command that writes to stderr.
func stderrCmd(msg string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("cmd /c echo %s 1>&2", msg)
	}
	return fmt.Sprintf("sh -c 'echo %s >&2'", msg)
}
