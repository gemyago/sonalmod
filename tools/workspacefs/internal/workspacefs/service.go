package workspacefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultMaxReadBytes is the default cap for a single read_text_file operation.
const DefaultMaxReadBytes int64 = 1024 * 1024

// DefaultMaxListEntries caps list_directory and directory_tree total entry/node counts.
const DefaultMaxListEntries = 100_000

// DefaultMaxTreeDepth is used when directory_tree omits max_depth.
const DefaultMaxTreeDepth = 20

// maxTreeDepthHardCap prevents unbounded recursion when callers pass very large max_depth.
const maxTreeDepthHardCap = 100

// WorkspaceMount describes one workspace directory to open in [NewService].
// Path must be an absolute path to an existing directory.
type WorkspaceMount struct {
	Identifier  string
	Description string
	Path        string
}

type workspaceEntry struct {
	identifier  string
	description string
	absPath     string
	root        *os.Root
}

// ExecConfig holds exec-specific configuration for the service.
type ExecConfig struct {
	MaxOutputBytes    int64
	DefaultTimeout    time.Duration
	MaxConcurrentJobs int
	BlockedCommands   []string
}

// backgroundJob tracks one running or completed background process.
type backgroundJob struct {
	cancel     func()
	stdout     *limitedBuffer
	stderr     *limitedBuffer
	startedAt  time.Time
	finishedAt time.Time
	exitCode   int
	done       chan struct{}
	running    bool
	mu         sync.Mutex
}

// Service holds opened [os.Root] handles keyed by workspace identifier.
type Service struct {
	entries        []workspaceEntry
	byID           map[string]*workspaceEntry
	maxReadBytes   int64
	maxListEntries int
	execEnabled    bool
	execConfig     ExecConfig
	jobsMu         sync.Mutex
	jobs           map[string]*backgroundJob
}

// ServiceOption configures [NewService].
type ServiceOption func(*Service)

// WithMaxReadBytes sets the maximum number of bytes read per file (must be positive).
func WithMaxReadBytes(n int64) ServiceOption {
	return func(s *Service) {
		if n > 0 {
			s.maxReadBytes = n
		}
	}
}

// WithMaxListEntries sets the maximum number of entries returned by list_directory or
// nodes materialized by directory_tree (must be positive).
func WithMaxListEntries(n int) ServiceOption {
	return func(s *Service) {
		if n > 0 {
			s.maxListEntries = n
		}
	}
}

// WithExecEnabled enables exec tools with the given configuration.
func WithExecEnabled(cfg ExecConfig) ServiceOption {
	return func(s *Service) {
		s.execEnabled = true
		s.execConfig = cfg
	}
}

// NewService opens an [os.Root] for each workspace mount. Identifiers must be unique
// (callers must validate); paths must be absolute and refer to existing directories.
func NewService(mounts []WorkspaceMount, opts ...ServiceOption) (*Service, error) {
	if len(mounts) == 0 {
		return nil, errors.New("workspacefs: no workspaces")
	}
	entries := make([]workspaceEntry, 0, len(mounts))
	byID := make(map[string]*workspaceEntry, len(mounts))
	for _, m := range mounts {
		abs := filepath.Clean(m.Path)
		root, err := os.OpenRoot(abs)
		if err != nil {
			for i := range entries {
				_ = entries[i].root.Close()
			}
			// Do not wrap OpenRoot errors: they include the absolute directory path.
			return nil, fmt.Errorf("workspacefs: cannot open workspace %q", m.Identifier)
		}
		e := workspaceEntry{
			identifier:  m.Identifier,
			description: m.Description,
			absPath:     abs,
			root:        root,
		}
		entries = append(entries, e)
		byID[m.Identifier] = &entries[len(entries)-1]
	}
	s := &Service{
		entries:        entries,
		byID:           byID,
		maxReadBytes:   DefaultMaxReadBytes,
		maxListEntries: DefaultMaxListEntries,
		jobs:           make(map[string]*backgroundJob),
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

// IsExecEnabled returns true when exec tools have been enabled.
func (s *Service) IsExecEnabled() bool {
	return s.execEnabled
}

// Close releases underlying [os.Root] handles and terminates active background jobs.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}

	// Cancel and drain all background jobs.
	s.jobsMu.Lock()
	jobs := make([]*backgroundJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	s.jobs = nil
	s.jobsMu.Unlock()

	for _, j := range jobs {
		j.cancel()
	}
	for _, j := range jobs {
		<-j.done
	}

	var firstErr error
	for i := range s.entries {
		if err := s.entries[i].root.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	s.entries = nil
	s.byID = nil
	return firstErr
}
