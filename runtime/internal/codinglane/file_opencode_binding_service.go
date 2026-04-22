package codinglane

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const openCodeBindingYAMLIndent = 2

// FileOpenCodeBindingService implements OpenCodeBindingService with file persistence.
// Each binding is stored at {baseDir}/opencode-bindings/{name}.yaml.
type FileOpenCodeBindingService struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Ensure FileOpenCodeBindingService implements OpenCodeBindingService.
var _ OpenCodeBindingService = (*FileOpenCodeBindingService)(nil)

// NewFileOpenCodeBindingService creates a file-backed OpenCode binding service.
func NewFileOpenCodeBindingService(
	baseDir string,
	logger *slog.Logger,
) (*FileOpenCodeBindingService, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, errors.New("base_dir is required")
	}

	dir := filepath.Join(baseDir, "opencode-bindings")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create opencode bindings dir: %w", err)
	}

	return &FileOpenCodeBindingService{
		baseDir: baseDir,
		logger:  logger,
	}, nil
}

func (s *FileOpenCodeBindingService) bindingPath(name string) string {
	return filepath.Join(s.baseDir, "opencode-bindings", name+".yaml")
}

func (s *FileOpenCodeBindingService) List(ctx context.Context) ([]OpenCodeBinding, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, "opencode-bindings")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []OpenCodeBinding{}, nil
		}
		return nil, fmt.Errorf("read opencode bindings dir: %w", err)
	}

	bindings := make([]OpenCodeBinding, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		binding, readErr := s.readBindingFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		bindings = append(bindings, binding)
	}

	sort.Slice(bindings, func(i, j int) bool {
		return bindings[i].CreatedAt.Before(bindings[j].CreatedAt)
	})

	return bindings, nil
}

func (s *FileOpenCodeBindingService) Get(ctx context.Context, name string) (*OpenCodeBinding, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	binding, err := s.readBindingFile(s.bindingPath(name))
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *FileOpenCodeBindingService) Create(
	ctx context.Context,
	params CreateOpenCodeBindingParams,
) (*OpenCodeBinding, error) {
	_ = ctx

	normalized, err := normalizeCreateOpenCodeBindingParams(params)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.bindingPath(normalized.Name)
	if _, err = os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrOpenCodeBindingNameConflict, normalized.Name)
	}

	now := time.Now().UTC()
	binding := OpenCodeBinding{
		Name:          normalized.Name,
		ProfileName:   normalized.ProfileName,
		CWD:           normalized.CWD,
		AgentCommand:  normalized.AgentCommand,
		LaunchOptions: normalized.LaunchOptions,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err = s.writeBindingFile(path, binding); err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *FileOpenCodeBindingService) Update(
	ctx context.Context,
	name string,
	params UpdateOpenCodeBindingParams,
) (*OpenCodeBinding, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.bindingPath(name)
	existing, err := s.readBindingFile(path)
	if err != nil {
		return nil, err
	}

	updated, err := applyOpenCodeBindingUpdate(existing, params)
	if err != nil {
		return nil, err
	}
	updated.UpdatedAt = time.Now().UTC()

	if err = s.writeBindingFile(path, updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *FileOpenCodeBindingService) Delete(ctx context.Context, name string) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.bindingPath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrOpenCodeBindingNotFound, name)
		}
		return fmt.Errorf("remove opencode binding file: %w", err)
	}
	return nil
}

// AutoMigrate is a no-op for file persistence.
func (s *FileOpenCodeBindingService) AutoMigrate() error {
	return nil
}

type openCodeAgentCommandStorage struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type openCodeLaunchOptionsStorage struct {
	Transport string `yaml:"transport"`
}

type openCodeBindingStorage struct {
	Name          string                       `yaml:"name"`
	ProfileName   string                       `yaml:"profileName"`
	CWD           string                       `yaml:"cwd"`
	AgentCommand  openCodeAgentCommandStorage  `yaml:"agentCommand"`
	LaunchOptions openCodeLaunchOptionsStorage `yaml:"launchOptions"`
	CreatedAt     time.Time                    `yaml:"createdAt"`
	UpdatedAt     time.Time                    `yaml:"updatedAt"`
}

func (s *FileOpenCodeBindingService) readBindingFile(path string) (OpenCodeBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			return OpenCodeBinding{}, fmt.Errorf("%w: %s", ErrOpenCodeBindingNotFound, name)
		}
		return OpenCodeBinding{}, fmt.Errorf("read opencode binding file: %w", err)
	}

	var stored openCodeBindingStorage
	if err = yaml.Unmarshal(data, &stored); err != nil {
		return OpenCodeBinding{}, fmt.Errorf("parse opencode binding file %s: %w", path, err)
	}

	return OpenCodeBinding{
		Name:        stored.Name,
		ProfileName: stored.ProfileName,
		CWD:         stored.CWD,
		AgentCommand: OpenCodeAgentCommand{
			Command: stored.AgentCommand.Command,
			Args:    stored.AgentCommand.Args,
		},
		LaunchOptions: OpenCodeLaunchOptions{
			Transport: stored.LaunchOptions.Transport,
		},
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
	}, nil
}

func (s *FileOpenCodeBindingService) writeBindingFile(path string, binding OpenCodeBinding) error {
	stored := openCodeBindingStorage{
		Name:        binding.Name,
		ProfileName: binding.ProfileName,
		CWD:         binding.CWD,
		AgentCommand: openCodeAgentCommandStorage{
			Command: binding.AgentCommand.Command,
			Args:    binding.AgentCommand.Args,
		},
		LaunchOptions: openCodeLaunchOptionsStorage{
			Transport: binding.LaunchOptions.Transport,
		},
		CreatedAt: binding.CreatedAt,
		UpdatedAt: binding.UpdatedAt,
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(openCodeBindingYAMLIndent)
	if err := enc.Encode(&stored); err != nil {
		return fmt.Errorf("marshal opencode binding: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("marshal opencode binding: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write opencode binding file: %w", err)
	}
	return nil
}
