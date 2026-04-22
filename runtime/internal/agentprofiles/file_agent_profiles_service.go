package agentprofiles

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

const agentProfileYAMLIndent = 2

// FileAgentProfilesService implements AgentProfilesService with file system persistence.
// Each profile is stored as a YAML file at {baseDir}/agent-profiles/{name}.yaml.
type FileAgentProfilesService struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Ensure FileAgentProfilesService implements AgentProfilesService.
var _ AgentProfilesService = (*FileAgentProfilesService)(nil)

// NewFileAgentProfilesService creates an AgentProfilesService that persists profiles
// as YAML files under {baseDir}/agent-profiles/.
func NewFileAgentProfilesService(baseDir string, logger *slog.Logger) (*FileAgentProfilesService, error) {
	if baseDir == "" {
		return nil, errors.New("base_dir is required")
	}

	profilesDir := filepath.Join(baseDir, "agent-profiles")
	if err := os.MkdirAll(profilesDir, 0750); err != nil {
		return nil, fmt.Errorf("create agent profiles dir: %w", err)
	}

	return &FileAgentProfilesService{
		baseDir: baseDir,
		logger:  logger,
	}, nil
}

func (s *FileAgentProfilesService) profilePath(name string) string {
	return filepath.Join(s.baseDir, "agent-profiles", name+".yaml")
}

func (s *FileAgentProfilesService) List(ctx context.Context) ([]AgentProfile, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	profilesDir := filepath.Join(s.baseDir, "agent-profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []AgentProfile{}, nil
		}
		return nil, fmt.Errorf("read agent profiles dir: %w", err)
	}

	profiles := make([]AgentProfile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		profile, readErr := s.readProfileFile(filepath.Join(profilesDir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].CreatedAt.Before(profiles[j].CreatedAt)
	})

	return profiles, nil
}

func (s *FileAgentProfilesService) Get(ctx context.Context, name string) (*AgentProfile, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	profile, err := s.readProfileFile(s.profilePath(name))
	if err != nil {
		return nil, err
	}

	return &profile, nil
}

func (s *FileAgentProfilesService) Create(
	ctx context.Context,
	params CreateAgentProfileParams,
) (*AgentProfile, error) {
	_ = ctx

	normalized, err := normalizeCreateParams(params)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.profilePath(normalized.Name)
	if _, err = os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrAgentProfileNameConflict, normalized.Name)
	}

	now := time.Now().UTC()
	profile := AgentProfile{
		Name:              normalized.Name,
		DisplayName:       normalized.DisplayName,
		Role:              normalized.Role,
		Instructions:      normalized.Instructions,
		ToolRefs:          normalized.ToolRefs,
		ExecutionSettings: normalized.ExecutionSettings,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err = s.writeProfileFile(path, profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

func (s *FileAgentProfilesService) Update(
	ctx context.Context,
	name string,
	params UpdateAgentProfileParams,
) (*AgentProfile, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.profilePath(name)
	existing, err := s.readProfileFile(path)
	if err != nil {
		return nil, err
	}

	updated, err := applyProfileUpdate(existing, params)
	if err != nil {
		return nil, err
	}
	updated.UpdatedAt = time.Now().UTC()

	if err = s.writeProfileFile(path, updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

func (s *FileAgentProfilesService) Delete(ctx context.Context, name string) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.profilePath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
		}
		return fmt.Errorf("remove agent profile file: %w", err)
	}

	return nil
}

// AutoMigrate is a no-op for file persistence.
func (s *FileAgentProfilesService) AutoMigrate() error {
	return nil
}

type executionSettingsStorage struct {
	DefaultModel string `yaml:"defaultModel"`
}

type agentProfileStorage struct {
	Name              string                   `yaml:"name"`
	DisplayName       string                   `yaml:"displayName"`
	Role              string                   `yaml:"role"`
	Instructions      string                   `yaml:"instructions"`
	ToolRefs          []string                 `yaml:"toolRefs"`
	ExecutionSettings executionSettingsStorage `yaml:"executionSettings"`
	CreatedAt         time.Time                `yaml:"createdAt"`
	UpdatedAt         time.Time                `yaml:"updatedAt"`
}

func (s *FileAgentProfilesService) readProfileFile(path string) (AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			return AgentProfile{}, fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
		}
		return AgentProfile{}, fmt.Errorf("read agent profile file: %w", err)
	}

	var stored agentProfileStorage
	if err = yaml.Unmarshal(data, &stored); err != nil {
		return AgentProfile{}, fmt.Errorf("parse agent profile file %s: %w", path, err)
	}

	return AgentProfile{
		Name:         stored.Name,
		DisplayName:  stored.DisplayName,
		Role:         stored.Role,
		Instructions: stored.Instructions,
		ToolRefs:     stored.ToolRefs,
		ExecutionSettings: ExecutionSettings{
			DefaultModel: stored.ExecutionSettings.DefaultModel,
		},
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
	}, nil
}

func (s *FileAgentProfilesService) writeProfileFile(path string, profile AgentProfile) error {
	stored := agentProfileStorage{
		Name:         profile.Name,
		DisplayName:  profile.DisplayName,
		Role:         profile.Role,
		Instructions: profile.Instructions,
		ToolRefs:     profile.ToolRefs,
		ExecutionSettings: executionSettingsStorage{
			DefaultModel: profile.ExecutionSettings.DefaultModel,
		},
		CreatedAt: profile.CreatedAt,
		UpdatedAt: profile.UpdatedAt,
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(agentProfileYAMLIndent)
	if err := enc.Encode(&stored); err != nil {
		return fmt.Errorf("marshal agent profile: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("marshal agent profile: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write agent profile file: %w", err)
	}
	return nil
}
