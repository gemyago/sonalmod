package llmproviders

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

const yamlIndent = 2

// FileProvidersConfigService implements ProvidersConfigService with file system persistence.
// Each provider config is stored as a YAML file at {baseDir}/providers/{name}.yaml
// (canonical). A same-named {name}.yml may exist for discovery and reads; writes use .yaml.
type FileProvidersConfigService struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Ensure FileProvidersConfigService implements ProvidersConfigService.
var _ ProvidersConfigService = (*FileProvidersConfigService)(nil)

// NewFileProvidersConfigService creates a ProvidersConfigService that persists provider
// configs as YAML under {baseDir}/providers/. The directory is created if it does not exist.
func NewFileProvidersConfigService(baseDir string, logger *slog.Logger) (*FileProvidersConfigService, error) {
	if baseDir == "" {
		return nil, errors.New("base_dir is required")
	}
	providersDir := filepath.Join(baseDir, "providers")
	if err := os.MkdirAll(providersDir, 0750); err != nil {
		return nil, fmt.Errorf("create providers dir: %w", err)
	}
	return &FileProvidersConfigService{
		baseDir: baseDir,
		logger:  logger,
	}, nil
}

// providerPath returns the canonical path for create and writes: {name}.yaml.
func (s *FileProvidersConfigService) providerPath(name string) string {
	return filepath.Join(s.baseDir, "providers", name+".yaml")
}

// resolveProviderReadPath returns the path to an existing provider file, preferring .yaml over .yml.
func (s *FileProvidersConfigService) resolveProviderReadPath(name string) (string, error) {
	yamlPath := filepath.Join(s.baseDir, "providers", name+".yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat provider file: %w", err)
	}
	ymlPath := filepath.Join(s.baseDir, "providers", name+".yml")
	if _, err := os.Stat(ymlPath); err == nil {
		return ymlPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat provider file: %w", err)
	}
	return "", fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
}

func (s *FileProvidersConfigService) List(ctx context.Context) ([]ProviderConfig, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.baseDir, "providers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProviderConfig{}, nil
		}
		return nil, fmt.Errorf("read providers dir: %w", err)
	}

	yamlByStem := make(map[string]string)
	ymlByStem := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		stem, ok := providerStemFromFileName(name)
		if !ok {
			continue
		}
		full := filepath.Join(dir, name)
		switch {
		case strings.HasSuffix(name, ".yaml"):
			yamlByStem[stem] = full
		case strings.HasSuffix(name, ".yml"):
			ymlByStem[stem] = full
		}
	}

	paths := make([]string, 0, len(yamlByStem)+len(ymlByStem))
	for _, p := range yamlByStem {
		paths = append(paths, p)
	}
	for stem, p := range ymlByStem {
		if _, hasYAML := yamlByStem[stem]; hasYAML {
			// Prefer foo.yaml when both foo.yaml and foo.yml exist.
			continue
		}
		paths = append(paths, p)
	}

	var configs []ProviderConfig
	for _, path := range paths {
		cfg, readErr := s.readProviderFile(path)
		if readErr != nil {
			return nil, readErr
		}
		configs = append(configs, cfg)
	}

	sort.Slice(configs, func(i, j int) bool {
		return configs[i].CreatedAt.Before(configs[j].CreatedAt)
	})

	return configs, nil
}

func (s *FileProvidersConfigService) Get(ctx context.Context, name string) (*ProviderConfig, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := s.resolveProviderReadPath(name)
	if err != nil {
		return nil, err
	}
	cfg, err := s.readProviderFile(path)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *FileProvidersConfigService) Create(
	ctx context.Context,
	params CreateProviderConfigParams,
) (*ProviderConfig, error) {
	_ = ctx

	if !providerNamePattern.MatchString(params.Name) {
		return nil, fmt.Errorf("invalid provider name %q: must match ^[a-z][a-z0-9-]*$", params.Name)
	}

	if params.Type != ProviderTypeOpenAICompatible {
		return nil, fmt.Errorf(
			"unsupported provider type %q: only %q is supported",
			params.Type,
			ProviderTypeOpenAICompatible,
		)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.providerPath(params.Name)
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderConfigNameConflict, params.Name)
	}
	ymlOnly := filepath.Join(s.baseDir, "providers", params.Name+".yml")
	if _, err := os.Stat(ymlOnly); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderConfigNameConflict, params.Name)
	}

	models := params.Models
	if models == nil {
		models = []ModelConfig{}
	}
	now := time.Now()
	cfg := ProviderConfig{
		Name:        params.Name,
		Type:        params.Type,
		DisplayName: params.DisplayName,
		BaseURL:     params.BaseURL,
		APIKey:      params.APIKey,
		Models:      models,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.writeProviderFile(path, cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *FileProvidersConfigService) Update(
	ctx context.Context,
	name string,
	params UpdateProviderConfigParams,
) (*ProviderConfig, error) {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	readPath, err := s.resolveProviderReadPath(name)
	if err != nil {
		return nil, err
	}
	existing, err := s.readProviderFile(readPath)
	if err != nil {
		return nil, err
	}

	writePath := s.providerPath(name)

	existing.DisplayName = params.DisplayName
	existing.BaseURL = params.BaseURL
	if params.APIKey != "" {
		existing.APIKey = params.APIKey
	}
	if params.Models != nil {
		existing.Models = params.Models
	} else {
		existing.Models = []ModelConfig{}
	}
	existing.UpdatedAt = time.Now()

	if writeErr := s.writeProviderFile(writePath, existing); writeErr != nil {
		return nil, writeErr
	}
	if readPath != writePath && strings.HasSuffix(readPath, ".yml") {
		if rmErr := os.Remove(readPath); rmErr != nil {
			return nil, fmt.Errorf("remove legacy .yml after migrate to .yaml: %w", rmErr)
		}
	}
	return &existing, nil
}

func (s *FileProvidersConfigService) Delete(ctx context.Context, name string) error {
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.resolveProviderReadPath(name)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
		}
		return fmt.Errorf("remove provider file: %w", err)
	}
	return nil
}

// modelFileStorage is the on-disk struct for a single model config.
type modelFileStorage struct {
	Name          string `yaml:"name"`
	DisplayName   string `yaml:"displayName"`
	Summarization bool   `yaml:"summarization"`
}

// providerFileStorage is the on-disk struct for provider config files (YAML).
type providerFileStorage struct {
	Name        string             `yaml:"name"`
	Type        string             `yaml:"type"`
	DisplayName string             `yaml:"displayName"`
	BaseURL     string             `yaml:"baseUrl"`
	APIKey      string             `yaml:"apiKey"      json:"-"`
	Models      []modelFileStorage `yaml:"models"`
	CreatedAt   time.Time          `yaml:"createdAt"`
	UpdatedAt   time.Time          `yaml:"updatedAt"`
}

func modelsToFileStorage(models []ModelConfig) []modelFileStorage {
	result := make([]modelFileStorage, len(models))
	for i, m := range models {
		result[i] = modelFileStorage(m)
	}
	return result
}

func modelsFromFileStorage(stored []modelFileStorage) []ModelConfig {
	result := make([]ModelConfig, len(stored))
	for i, m := range stored {
		result[i] = ModelConfig(m)
	}
	return result
}

func providerStemFromFileName(name string) (string, bool) {
	switch {
	case strings.HasSuffix(name, ".yaml"):
		return strings.TrimSuffix(name, ".yaml"), true
	case strings.HasSuffix(name, ".yml"):
		return strings.TrimSuffix(name, ".yml"), true
	default:
		return "", false
	}
}

func providerNameFromFilePath(path string) string {
	base := filepath.Base(path)
	switch {
	case strings.HasSuffix(base, ".yaml"):
		return strings.TrimSuffix(base, ".yaml")
	case strings.HasSuffix(base, ".yml"):
		return strings.TrimSuffix(base, ".yml")
	default:
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
}

func (s *FileProvidersConfigService) readProviderFile(path string) (ProviderConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := providerNameFromFilePath(path)
			return ProviderConfig{}, fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
		}
		return ProviderConfig{}, fmt.Errorf("read provider file: %w", err)
	}

	var stored providerFileStorage
	if err = yaml.Unmarshal(data, &stored); err != nil {
		return ProviderConfig{}, fmt.Errorf("parse provider file %s: %w", path, err)
	}

	return ProviderConfig{
		Name:        stored.Name,
		Type:        stored.Type,
		DisplayName: stored.DisplayName,
		BaseURL:     stored.BaseURL,
		APIKey:      stored.APIKey,
		Models:      modelsFromFileStorage(stored.Models),
		CreatedAt:   stored.CreatedAt,
		UpdatedAt:   stored.UpdatedAt,
	}, nil
}

func (s *FileProvidersConfigService) writeProviderFile(path string, cfg ProviderConfig) error {
	stored := providerFileStorage{
		Name:        cfg.Name,
		Type:        cfg.Type,
		DisplayName: cfg.DisplayName,
		BaseURL:     cfg.BaseURL,
		APIKey:      cfg.APIKey,
		Models:      modelsToFileStorage(cfg.Models),
		CreatedAt:   cfg.CreatedAt,
		UpdatedAt:   cfg.UpdatedAt,
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(&stored); err != nil {
		return fmt.Errorf("marshal provider config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("marshal provider config: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write provider file: %w", err)
	}
	return nil
}
