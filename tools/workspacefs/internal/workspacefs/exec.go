package workspacefs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExecCommandRequest is the request model for workspacefs_exec_command.
type ExecCommandRequest struct {
	Workspace        string `json:"workspace"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	Background       bool   `json:"background,omitempty"`
}

// ExecCommandResponse is the response model for workspacefs_exec_command.
type ExecCommandResponse struct {
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timedOut"`
	JobID     string `json:"jobId,omitempty"`
	Running   bool   `json:"running,omitempty"`
}

// ExecJobOutputRequest is the request model for workspacefs_exec_job_output.
type ExecJobOutputRequest struct {
	Workspace string `json:"workspace"`
	JobID     string `json:"jobId"`
}

// ExecJobOutputResponse is the response model for workspacefs_exec_job_output.
type ExecJobOutputResponse struct {
	Running   bool   `json:"running"`
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
	Elapsed   string `json:"elapsed,omitempty"`
}

// ExecKillJobRequest is the request model for workspacefs_exec_kill_job.
type ExecKillJobRequest struct {
	Workspace string `json:"workspace"`
	JobID     string `json:"jobId"`
}

// ExecKillJobResponse is the response model for workspacefs_exec_kill_job.
type ExecKillJobResponse struct {
	Killed bool `json:"killed"`
}

// builtinBlockedCommands returns the built-in denylist applied when ExecConfig.BlockedCommands is nil.
// It covers common network exfiltration and system-mutation tools.
func builtinBlockedCommands() []string {
	return []string{
		"curl",
		"wget",
		"nc",
		"netcat",
		"ncat",
		"ssh",
		"scp",
		"sftp",
		"ftp",
		"telnet",
		"rsync",
	}
}

// ExecCommand executes a shell command within the given workspace.
// When Background is false the command runs in the foreground and returns when it completes (or times out).
// When Background is true the command starts asynchronously and a job ID is returned immediately.
func (s *Service) ExecCommand(ctx context.Context, in ExecCommandRequest) (ExecCommandResponse, error) {
	if !s.execEnabled {
		return ExecCommandResponse{}, errors.New("workspacefs: exec tools are not enabled")
	}

	entry, err := s.pickWorkspaceEntry(in.Workspace)
	if err != nil {
		return ExecCommandResponse{}, err
	}

	workDir, err := resolveWorkingDirectory(entry.absPath, in.WorkingDirectory)
	if err != nil {
		return ExecCommandResponse{}, err
	}

	if strings.TrimSpace(in.Command) == "" {
		return ExecCommandResponse{}, errors.New("workspacefs: command is required")
	}

	if policyErr := s.checkCommandPolicy(in.Command); policyErr != nil {
		return ExecCommandResponse{}, policyErr
	}

	if in.Background {
		return s.startBackgroundJob(ctx, in.Command, workDir)
	}

	return s.runForeground(ctx, in.Command, workDir)
}

// checkCommandPolicy returns an error if the command matches a blocked prefix.
// It uses ExecConfig.BlockedCommands when non-nil, or the built-in default denylist otherwise.
func (s *Service) checkCommandPolicy(command string) error {
	blocked := s.execConfig.BlockedCommands
	if blocked == nil {
		blocked = builtinBlockedCommands()
	}
	trimmed := strings.TrimSpace(command)
	// Extract the first token (the program name) from the command string.
	first := strings.Fields(trimmed)[0]
	// Strip any path prefix so "/usr/bin/curl" is treated like "curl".
	first = filepath.Base(first)
	lc := strings.ToLower(first)
	for _, b := range blocked {
		if lc == strings.ToLower(b) {
			return fmt.Errorf("workspacefs: command %q is blocked by policy", first)
		}
	}
	return nil
}

// runForeground executes a command synchronously and returns the result.
func (s *Service) runForeground(ctx context.Context, command, workDir string) (ExecCommandResponse, error) {
	timeout := s.execConfig.DefaultTimeout
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCommand(command)
	configureExecCommand(cmd)
	cmd.Dir = workDir

	maxBytes := s.execConfig.MaxOutputBytes

	var stdoutBuf, stderrBuf limitedBuffer
	stdoutBuf.limit = maxBytes
	stderrBuf.limit = maxBytes
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return ExecCommandResponse{}, fmt.Errorf("workspacefs: exec failed: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	var runErr error
	select {
	case <-cmdCtx.Done():
		terminateCmdProcess(cmd)
		runErr = <-waitCh
	case err := <-waitCh:
		runErr = err
	}

	timedOut := errors.Is(cmdCtx.Err(), context.DeadlineExceeded)

	resp := ExecCommandResponse{
		Stdout:    stdoutBuf.String(),
		Stderr:    stderrBuf.String(),
		Truncated: stdoutBuf.truncated || stderrBuf.truncated,
		TimedOut:  timedOut,
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = -1
		}
	}

	return resp, nil
}

// startBackgroundJob starts a command asynchronously and registers it as a background job.
func (s *Service) startBackgroundJob(_ context.Context, command, workDir string) (ExecCommandResponse, error) {
	s.jobsMu.Lock()
	if len(s.jobs) >= s.execConfig.MaxConcurrentJobs {
		s.jobsMu.Unlock()
		return ExecCommandResponse{}, fmt.Errorf(
			"workspacefs: max concurrent background jobs (%d) reached", s.execConfig.MaxConcurrentJobs,
		)
	}

	jobID := uuid.NewString()

	maxBytes := s.execConfig.MaxOutputBytes
	job := &backgroundJob{
		done:      make(chan struct{}),
		running:   true,
		startedAt: time.Now(),
		stdout:    &limitedBuffer{limit: maxBytes},
		stderr:    &limitedBuffer{limit: maxBytes},
	}

	cmd := buildShellCommand(command)
	configureExecCommand(cmd)
	cmd.Dir = workDir
	cmd.Stdout = job.stdout
	cmd.Stderr = job.stderr

	var timer *time.Timer
	job.cancel = func() {
		if timer != nil {
			timer.Stop()
		}
		terminateCmdProcess(cmd)
	}

	s.jobs[jobID] = job
	s.jobsMu.Unlock()

	if err := cmd.Start(); err != nil {
		job.cancel()
		s.jobsMu.Lock()
		delete(s.jobs, jobID)
		s.jobsMu.Unlock()
		close(job.done)
		return ExecCommandResponse{}, fmt.Errorf("workspacefs: failed to start background job: %w", err)
	}

	timer = time.AfterFunc(s.execConfig.DefaultTimeout, job.cancel)

	go func() {
		defer close(job.done)
		defer func() {
			if timer != nil {
				timer.Stop()
			}
		}()

		runErr := cmd.Wait()

		job.mu.Lock()
		defer job.mu.Unlock()
		job.running = false
		job.finishedAt = time.Now()
		if runErr != nil {
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				job.exitCode = exitErr.ExitCode()
			} else {
				job.exitCode = -1
			}
		}
	}()

	return ExecCommandResponse{
		JobID:   jobID,
		Running: true,
	}, nil
}

// ExecJobOutput returns output and status for a background job.
func (s *Service) ExecJobOutput(_ context.Context, in ExecJobOutputRequest) (ExecJobOutputResponse, error) {
	if !s.execEnabled {
		return ExecJobOutputResponse{}, errors.New("workspacefs: exec tools are not enabled")
	}

	s.jobsMu.Lock()
	job, ok := s.jobs[in.JobID]
	s.jobsMu.Unlock()

	if !ok {
		return ExecJobOutputResponse{}, fmt.Errorf("workspacefs: job %q not found", in.JobID)
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	resp := ExecJobOutputResponse{
		Running:   job.running,
		ExitCode:  job.exitCode,
		Stdout:    job.stdout.String(),
		Stderr:    job.stderr.String(),
		Truncated: job.stdout.truncated || job.stderr.truncated,
	}
	if !job.running {
		resp.Elapsed = job.finishedAt.Sub(job.startedAt).String()
	}
	return resp, nil
}

// ExecKillJob terminates a background job.
func (s *Service) ExecKillJob(_ context.Context, in ExecKillJobRequest) (ExecKillJobResponse, error) {
	if !s.execEnabled {
		return ExecKillJobResponse{}, errors.New("workspacefs: exec tools are not enabled")
	}

	s.jobsMu.Lock()
	job, ok := s.jobs[in.JobID]
	s.jobsMu.Unlock()

	if !ok {
		return ExecKillJobResponse{Killed: false}, nil
	}

	job.mu.Lock()
	running := job.running
	job.mu.Unlock()

	if !running {
		return ExecKillJobResponse{Killed: false}, nil
	}

	job.cancel()
	<-job.done
	return ExecKillJobResponse{Killed: true}, nil
}

// resolveWorkingDirectory validates and resolves the working directory relative to the workspace root.
// An empty workingDir uses the workspace root itself.
func resolveWorkingDirectory(workspaceAbsPath, workingDir string) (string, error) {
	if workingDir == "" {
		return workspaceAbsPath, nil
	}
	if filepath.IsAbs(workingDir) {
		return "", errors.New("workspacefs: working directory must be relative to workspace")
	}
	clean := filepath.ToSlash(filepath.Clean(workingDir))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("workspacefs: working directory escapes workspace")
	}
	return filepath.Join(workspaceAbsPath, filepath.FromSlash(clean)), nil
}

// buildShellCommand constructs a shell-wrapped command for the current OS.
// A background [context.Context] is used to satisfy static checks; cancellation
// and timeouts are enforced via [terminateCmdProcess] and explicit deadlines.
func buildShellCommand(command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(context.Background(), "cmd", "/c", command)
	}
	return exec.CommandContext(context.Background(), "sh", "-c", command)
}

// limitedBuffer is an [io.Writer] that captures up to limit bytes and sets truncated when exceeded.
type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int64
	written   int64
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.written
	if remaining <= 0 {
		b.truncated = true
		return n, nil
	}
	if int64(len(p)) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	written, err := b.buf.Write(p)
	b.written += int64(written)
	// Return the full length so exec doesn't error on short write.
	return n, err
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}

// Ensure limitedBuffer satisfies [io.Writer].
var _ io.Writer = (*limitedBuffer)(nil)
