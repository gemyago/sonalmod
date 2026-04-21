package workspacefs

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gemyago/sonalmod/runtime/agent"
	ifs "github.com/gemyago/sonalmod/tools/workspacefs/internal/workspacefs"
)

// ExpectedToolCount is the number of workspace filesystem tools registered when RegisterTools succeeds
// and exec tools are not enabled.
const ExpectedToolCount = 9

// ExpectedToolCountWithExec is the number of workspace filesystem tools registered when RegisterTools
// succeeds with exec tools enabled (base tools + 3 exec tools).
const ExpectedToolCountWithExec = ExpectedToolCount + 3

// DefaultExecMaxOutputBytes is the default maximum bytes captured per command output stream.
const DefaultExecMaxOutputBytes int64 = 1024 * 1024 // 1 MiB

// DefaultExecTimeout is the default per-command timeout for foreground execution.
const DefaultExecTimeout = 30 * time.Second

// DefaultExecMaxConcurrentJobs is the default cap on background jobs running simultaneously.
const DefaultExecMaxConcurrentJobs = 10

// ExecOptions holds exec-specific limits and policy for RegisterTools.
// Zero values for positive numeric fields use safe built-in defaults.
type ExecOptions struct {
	// MaxOutputBytes caps stdout+stderr bytes captured per command (must be non-negative; 0 uses default).
	MaxOutputBytes int64

	// DefaultTimeout is the per-command timeout for foreground execution (must be non-negative; 0 uses default).
	DefaultTimeout time.Duration

	// MaxConcurrentJobs caps the number of background jobs (must be non-negative; 0 uses default).
	MaxConcurrentJobs int

	// BlockedCommands lists command name prefixes that are denied before execution.
	// Nil/empty uses the built-in safe defaults.
	BlockedCommands []string
}

// WorkspaceConfig binds a model-visible workspace identifier to a host directory.
// Identifier and Description are shown to models; Path is validated at registration and kept internal.
//
// Identifier should be short and consistent (for example "user-docs", "codebase"); the module does not enforce a strict pattern.
type WorkspaceConfig struct {
	Identifier  string
	Description string
	Path        string
}

type registerToolsOpts struct {
	workspaces  []WorkspaceConfig
	logger      *slog.Logger
	execEnabled bool
	execOptions ExecOptions
}

// RegisterToolsOpt configures RegisterTools.
type RegisterToolsOpt func(*registerToolsOpts)

// WithWorkspaces sets workspace entries (identifier, description, directory path). Each identifier must be unique;
// paths must exist and be directories. Replaces any prior WithWorkspaces slice from an earlier option.
func WithWorkspaces(workspaces []WorkspaceConfig) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.workspaces = append([]WorkspaceConfig(nil), workspaces...)
	}
}

// WithLogger sets the logger used for registration diagnostics.
// Nil falls back to [slog.Default] (same as omitting WithLogger).
func WithLogger(logger *slog.Logger) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.logger = logger
	}
}

// WithExec enables exec tools with the given options. Exec tools are disabled by default.
// Zero values for positive numeric fields in ExecOptions use safe built-in defaults.
func WithExec(execOpts ExecOptions) RegisterToolsOpt {
	return func(opts *registerToolsOpts) {
		opts.execEnabled = true
		opts.execOptions = execOpts
	}
}

// ToolsRegistry is the minimal surface needed to register agent tools (mirrors tools/firecrawl).
type ToolsRegistry interface {
	AddTools(tools ...agent.DefinedTool)
}

// RegisterTools registers workspace filesystem tools when configuration is valid.
// It returns an error when workspace configuration cannot be validated or services cannot be created.
func RegisterTools(registry ToolsRegistry, opts ...RegisterToolsOpt) error {
	o := registerToolsOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.logger == nil {
		o.logger = slog.Default()
	}
	mounts, err := validateWorkspaces(o.workspaces)
	if err != nil {
		o.logger.Debug("workspacefs: invalid workspace configuration", "err", err)
		return err
	}
	if o.execEnabled {
		if execErr := validateExecOptions(o.execOptions); execErr != nil {
			o.logger.Debug("workspacefs: invalid exec options", "err", execErr)
			return execErr
		}
	}
	svcOpts := buildServiceOptions(o)
	svc, err := ifs.NewService(mounts, svcOpts...)
	if err != nil {
		wrapped := fmt.Errorf("workspacefs: failed to open workspace directories: %w", err)
		o.logger.Debug("workspacefs: failed to open workspace directories", "err", wrapped)
		return wrapped
	}
	registry.AddTools(workspacefsAgentTools(svc, o.logger)...)
	return nil
}

func validateExecOptions(o ExecOptions) error {
	if o.MaxOutputBytes < 0 {
		return errors.New("workspacefs: exec option MaxOutputBytes must be non-negative")
	}
	if o.DefaultTimeout < 0 {
		return errors.New("workspacefs: exec option DefaultTimeout must be non-negative")
	}
	if o.MaxConcurrentJobs < 0 {
		return errors.New("workspacefs: exec option MaxConcurrentJobs must be non-negative")
	}
	return nil
}

func buildServiceOptions(o registerToolsOpts) []ifs.ServiceOption {
	var svcOpts []ifs.ServiceOption
	if o.execEnabled {
		execOpts := o.execOptions
		if execOpts.MaxOutputBytes == 0 {
			execOpts.MaxOutputBytes = DefaultExecMaxOutputBytes
		}
		if execOpts.DefaultTimeout == 0 {
			execOpts.DefaultTimeout = DefaultExecTimeout
		}
		if execOpts.MaxConcurrentJobs == 0 {
			execOpts.MaxConcurrentJobs = DefaultExecMaxConcurrentJobs
		}
		svcOpts = append(svcOpts, ifs.WithExecEnabled(ifs.ExecConfig{
			MaxOutputBytes:    execOpts.MaxOutputBytes,
			DefaultTimeout:    execOpts.DefaultTimeout,
			MaxConcurrentJobs: execOpts.MaxConcurrentJobs,
			BlockedCommands:   execOpts.BlockedCommands,
		}))
	}
	return svcOpts
}

func validateWorkspaces(workspaces []WorkspaceConfig) ([]ifs.WorkspaceMount, error) {
	if len(workspaces) == 0 {
		return nil, errors.New("workspacefs: at least one workspace is required")
	}
	seen := make(map[string]struct{}, len(workspaces))
	out := make([]ifs.WorkspaceMount, 0, len(workspaces))
	for _, w := range workspaces {
		id := strings.TrimSpace(w.Identifier)
		if id == "" {
			return nil, errors.New("workspacefs: workspace identifier is required")
		}
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("workspacefs: duplicate workspace identifier %q", id)
		}
		seen[id] = struct{}{}

		desc := strings.TrimSpace(w.Description)
		if desc == "" {
			return nil, fmt.Errorf("workspacefs: workspace description is required for workspace %q", id)
		}

		p := strings.TrimSpace(w.Path)
		if p == "" {
			return nil, fmt.Errorf("workspacefs: path is required for workspace %q", id)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			// Do not wrap OS errors: they may include absolute paths from user input.
			return nil, fmt.Errorf("workspacefs: invalid path for workspace %q", id)
		}
		st, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("workspacefs: path for workspace %q does not exist", id)
			}
			return nil, fmt.Errorf("workspacefs: path for workspace %q is not accessible", id)
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("workspacefs: path for workspace %q is not a directory", id)
		}
		out = append(out, ifs.WorkspaceMount{
			Identifier:  id,
			Description: desc,
			Path:        filepath.Clean(abs),
		})
	}
	return out, nil
}
