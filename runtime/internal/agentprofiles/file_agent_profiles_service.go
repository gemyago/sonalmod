package agentprofiles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	agentProfileYAMLIndent  = 2
	agentProfilesDirName    = "agents"
	agentProfileFileExt     = ".md"
	profileFrontmatterDelim = "---"
	profileMarkdownMatches  = 3
)

var profileMarkdownPattern = regexp.MustCompile(
	`(?s)^---[ \t]*\r?\n(.*?)\r?\n---[ \t]*\r?\n?(.*)$`,
)

// FileAgentProfilesService implements AgentProfilesService with file system persistence.
// Each profile is stored as a Markdown file at {baseDir}/agents/{name}.md.
type FileAgentProfilesService struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Ensure FileAgentProfilesService implements AgentProfilesService.
var _ AgentProfilesService = (*FileAgentProfilesService)(nil)

// NewFileAgentProfilesService creates an AgentProfilesService that persists profiles
// as Markdown files under {baseDir}/agents/.
func NewFileAgentProfilesService(baseDir string, logger *slog.Logger) (*FileAgentProfilesService, error) {
	if baseDir == "" {
		return nil, errors.New("base_dir is required")
	}

	profilesDir := filepath.Join(baseDir, agentProfilesDirName)
	if err := os.MkdirAll(profilesDir, 0750); err != nil {
		return nil, fmt.Errorf("create agent profiles dir: %w", err)
	}

	return &FileAgentProfilesService{
		baseDir: baseDir,
		logger:  logger,
	}, nil
}

func (s *FileAgentProfilesService) profilesDir() string {
	return filepath.Join(s.baseDir, agentProfilesDirName)
}

func (s *FileAgentProfilesService) profilePath(name string) string {
	return filepath.Join(s.profilesDir(), name+agentProfileFileExt)
}

func (s *FileAgentProfilesService) List(ctx context.Context) ([]AgentProfile, error) {
	_ = ctx

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.profilesDir())
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
		if filepath.Ext(entry.Name()) != agentProfileFileExt {
			continue
		}

		profile, readErr := s.readProfileFile(filepath.Join(s.profilesDir(), entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		profiles = append(profiles, profile)
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].CreatedAt.Equal(profiles[j].CreatedAt) {
			return profiles[i].Name < profiles[j].Name
		}
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
	if _, statErr := os.Stat(path); statErr == nil {
		return nil, fmt.Errorf("%w: %s", ErrAgentProfileNameConflict, normalized.Name)
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat agent profile file: %w", statErr)
	}

	profile := AgentProfile{
		Name:              normalized.Name,
		DisplayName:       normalized.DisplayName,
		Role:              normalized.Role,
		Instructions:      normalized.Instructions,
		ToolRefs:          normalized.ToolRefs,
		ExecutionSettings: normalized.ExecutionSettings,
	}
	if err = s.writeProfileFile(path, profile); err != nil {
		return nil, err
	}
	createdAt, updatedAt, err := profileTimestampsFromPath(path)
	if err != nil {
		return nil, err
	}
	profile.CreatedAt = createdAt
	profile.UpdatedAt = updatedAt
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

	originalCreatedAt := existing.CreatedAt

	updated, err := applyProfileUpdate(existing, params)
	if err != nil {
		return nil, err
	}
	if err = s.writeProfileFile(path, updated); err != nil {
		return nil, err
	}
	_, updatedAt, err := profileTimestampsFromPath(path)
	if err != nil {
		return nil, err
	}
	updated.CreatedAt = originalCreatedAt
	updated.UpdatedAt = updatedAt
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

type agentProfileFrontmatter struct {
	Name              string                            `yaml:"name"`
	DisplayName       string                            `yaml:"displayName,omitempty"`
	Role              string                            `yaml:"role"`
	Description       string                            `yaml:"description,omitempty"`
	Model             string                            `yaml:"model,omitempty"`
	Tools             []string                          `yaml:"tools"`
	ExecutionSettings *agentProfileExecutionFrontmatter `yaml:"executionSettings,omitempty"`
}

type agentProfileExecutionFrontmatter struct {
	Mode         ExecutionMode                        `yaml:"mode,omitempty"`
	DefaultModel string                               `yaml:"defaultModel,omitempty"`
	AgentCommand *agentProfileAgentCommandFrontmatter `yaml:"agentCommand,omitempty"`
	Cwd          string                               `yaml:"cwd,omitempty"`
}

type agentProfileAgentCommandFrontmatter struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
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

	profile, err := parseProfileMarkdown(path, data)
	if err != nil {
		return AgentProfile{}, err
	}

	profile.CreatedAt, profile.UpdatedAt, err = profileTimestampsFromPath(path)
	if err != nil {
		return AgentProfile{}, err
	}

	return profile, nil
}

func parseProfileMarkdown(path string, data []byte) (AgentProfile, error) {
	matches := profileMarkdownPattern.FindSubmatch(data)
	if len(matches) != profileMarkdownMatches {
		return AgentProfile{}, fmt.Errorf(
			"parse agent profile file %s: expected markdown with YAML frontmatter delimited by %q",
			path,
			profileFrontmatterDelim,
		)
	}

	var frontmatter agentProfileFrontmatter
	if err := yaml.Unmarshal(matches[1], &frontmatter); err != nil {
		return AgentProfile{}, fmt.Errorf("parse agent profile frontmatter %s: %w", path, err)
	}

	if strings.TrimSpace(frontmatter.Name) == "" {
		return AgentProfile{}, fmt.Errorf(
			"validate agent profile file %s: missing required frontmatter field `name`",
			path,
		)
	}
	if strings.TrimSpace(frontmatter.Role) == "" {
		return AgentProfile{}, fmt.Errorf(
			"validate agent profile file %s: missing required frontmatter field `role`",
			path,
		)
	}
	if strings.TrimSpace(frontmatter.Model) == "" && frontmatter.ExecutionSettings == nil {
		return AgentProfile{}, fmt.Errorf(
			"validate agent profile file %s: missing required frontmatter field `model` or `executionSettings`",
			path,
		)
	}

	instructions := strings.TrimSpace(string(matches[2]))
	if instructions == "" {
		return AgentProfile{}, fmt.Errorf(
			"validate agent profile file %s: markdown body must contain instructions",
			path,
		)
	}

	expectedName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.TrimSpace(frontmatter.Name) != expectedName {
		return AgentProfile{}, fmt.Errorf(
			"validate agent profile file %s: frontmatter name %q must match file name %q",
			path,
			frontmatter.Name,
			expectedName,
		)
	}

	execSettings := ExecutionSettings{}
	if frontmatter.ExecutionSettings == nil {
		execSettings.DefaultModel = frontmatter.Model
	} else {
		execSettings.Mode = frontmatter.ExecutionSettings.Mode
		execSettings.DefaultModel = frontmatter.ExecutionSettings.DefaultModel
		execSettings.Cwd = frontmatter.ExecutionSettings.Cwd
		if frontmatter.ExecutionSettings.AgentCommand != nil {
			execSettings.AgentCommand = ACPStdioAgentCommand{
				Command: frontmatter.ExecutionSettings.AgentCommand.Command,
				Args:    append([]string(nil), frontmatter.ExecutionSettings.AgentCommand.Args...),
			}
		}
	}

	normalized, err := normalizeCreateParams(CreateAgentProfileParams{
		Name:              frontmatter.Name,
		DisplayName:       frontmatter.DisplayName,
		Role:              frontmatter.Role,
		Instructions:      instructions,
		ToolRefs:          frontmatter.Tools,
		ExecutionSettings: execSettings,
	})
	if err != nil {
		return AgentProfile{}, fmt.Errorf("validate agent profile file %s: %w", path, err)
	}

	return AgentProfile{
		Name:              normalized.Name,
		DisplayName:       normalized.DisplayName,
		Role:              normalized.Role,
		Instructions:      normalized.Instructions,
		ToolRefs:          normalized.ToolRefs,
		ExecutionSettings: normalized.ExecutionSettings,
	}, nil
}

func (s *FileAgentProfilesService) writeProfileFile(path string, profile AgentProfile) error {
	frontmatter := agentProfileFrontmatter{
		Name:        profile.Name,
		DisplayName: profile.DisplayName,
		Role:        profile.Role,
		Tools:       append([]string(nil), profile.ToolRefs...),
	}

	switch profile.ExecutionSettings.ModeOrDefault() {
	case ExecutionModeRegular:
		if profile.ExecutionSettings.Mode == "" {
			frontmatter.Model = profile.ExecutionSettings.DefaultModel
		} else {
			frontmatter.ExecutionSettings = &agentProfileExecutionFrontmatter{
				Mode:         profile.ExecutionSettings.Mode,
				DefaultModel: profile.ExecutionSettings.DefaultModel,
			}
		}
	case ExecutionModeACPStdio:
		frontmatter.ExecutionSettings = &agentProfileExecutionFrontmatter{
			Mode: profile.ExecutionSettings.Mode,
			AgentCommand: &agentProfileAgentCommandFrontmatter{
				Command: profile.ExecutionSettings.AgentCommand.Command,
				Args:    append([]string(nil), profile.ExecutionSettings.AgentCommand.Args...),
			},
			Cwd: profile.ExecutionSettings.Cwd,
		}
	}

	var frontmatterBuf bytes.Buffer
	enc := yaml.NewEncoder(&frontmatterBuf)
	enc.SetIndent(agentProfileYAMLIndent)
	if err := enc.Encode(frontmatter); err != nil {
		return fmt.Errorf("marshal agent profile frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("marshal agent profile frontmatter: %w", err)
	}

	instructions := strings.TrimSpace(profile.Instructions)

	var markdownBuf bytes.Buffer
	markdownBuf.WriteString(profileFrontmatterDelim)
	markdownBuf.WriteString("\n")
	markdownBuf.Write(bytes.TrimSpace(frontmatterBuf.Bytes()))
	markdownBuf.WriteString("\n")
	markdownBuf.WriteString(profileFrontmatterDelim)
	markdownBuf.WriteString("\n")
	markdownBuf.WriteString(instructions)
	markdownBuf.WriteString("\n")

	if err := os.WriteFile(path, markdownBuf.Bytes(), 0600); err != nil {
		return fmt.Errorf("write agent profile file: %w", err)
	}
	return nil
}

func profileTimestampsFromFileInfo(info os.FileInfo) (time.Time, time.Time) {
	updatedAt := info.ModTime().UTC()
	createdAt := creationTimeFromFileInfo(info)
	if createdAt.IsZero() {
		createdAt = updatedAt
	}
	if createdAt.After(updatedAt) {
		createdAt = updatedAt
	}
	return createdAt, updatedAt
}

func profileTimestampsFromPath(path string) (time.Time, time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			return time.Time{}, time.Time{}, fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
		}
		return time.Time{}, time.Time{}, fmt.Errorf("stat agent profile file: %w", err)
	}
	createdAt, updatedAt := profileTimestampsFromFileInfo(info)
	return createdAt, updatedAt, nil
}

func creationTimeFromFileInfo(info os.FileInfo) time.Time {
	sys := info.Sys()
	if sys == nil {
		return time.Time{}
	}

	value := reflect.ValueOf(sys)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return time.Time{}
	}

	if birthtime := timeFromTimespecField(value.FieldByName("Birthtimespec")); !birthtime.IsZero() {
		return birthtime
	}
	if birthtime := timeFromTimespecField(value.FieldByName("Birthtim")); !birthtime.IsZero() {
		return birthtime
	}
	if birthtime := timeFromWindowsCreationField(value.FieldByName("CreationTime")); !birthtime.IsZero() {
		return birthtime
	}

	return time.Time{}
}

func timeFromTimespecField(value reflect.Value) time.Time {
	if !value.IsValid() {
		return time.Time{}
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return time.Time{}
	}

	secField := value.FieldByName("Sec")
	nsecField := value.FieldByName("Nsec")
	sec, secOK := reflectAsInt64(secField)
	nsec, nsecOK := reflectAsInt64(nsecField)
	if !secOK || !nsecOK || sec <= 0 || nsec < 0 || nsec >= 1e9 {
		return time.Time{}
	}
	return time.Unix(sec, nsec).UTC()
}

func timeFromWindowsCreationField(value reflect.Value) time.Time {
	if !value.IsValid() {
		return time.Time{}
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return time.Time{}
		}
		value = value.Elem()
	}
	method := value.MethodByName("Nanoseconds")
	if !method.IsValid() && value.CanAddr() {
		method = value.Addr().MethodByName("Nanoseconds")
	}
	if !method.IsValid() {
		return time.Time{}
	}
	methodType := method.Type()
	if methodType.NumIn() != 0 || methodType.NumOut() != 1 || methodType.Out(0).Kind() != reflect.Int64 {
		return time.Time{}
	}
	outputs := method.Call(nil)
	nanos := outputs[0].Int()
	if nanos <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

func reflectAsInt64(value reflect.Value) (int64, bool) {
	kind := value.Kind()
	if kind >= reflect.Int && kind <= reflect.Int64 {
		if value.CanInt() {
			return value.Int(), true
		}
		return 0, false
	}
	if kind >= reflect.Uint && kind <= reflect.Uintptr {
		if value.CanUint() {
			uintValue := value.Uint()
			if uintValue > math.MaxInt64 {
				return 0, false
			}
			return int64(uintValue), true
		}
		return 0, false
	}
	return 0, false
}
