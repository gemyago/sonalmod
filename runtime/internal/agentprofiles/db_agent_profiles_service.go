package agentprofiles

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/gormsonal"
	"gorm.io/gorm"
)

type dbExecutionSettings struct {
	Mode         ExecutionMode           `json:"mode,omitempty"`
	DefaultModel string                  `json:"defaultModel,omitempty"`
	AgentCommand *dbACPStdioAgentCommand `json:"agentCommand,omitempty"`
	Cwd          string                  `json:"cwd,omitempty"`
}

type dbACPStdioAgentCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// agentProfileModel is the GORM model for persisted agent profiles.
type agentProfileModel struct {
	Name              string              `gorm:"column:name;primaryKey;size:255"`
	DisplayName       string              `gorm:"column:display_name;size:255"`
	Role              string              `gorm:"column:role;size:255;not null"`
	Instructions      string              `gorm:"column:instructions;type:text;not null"`
	ToolRefs          []string            `gorm:"column:tool_refs;serializer:json"`
	ExecutionSettings dbExecutionSettings `gorm:"column:execution_settings;serializer:json"`
	CreatedAt         time.Time           `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time           `gorm:"column:updated_at;autoUpdateTime"`
}

func (agentProfileModel) TableName() string { return "agent_profiles" }

// DatabaseAgentProfilesService implements AgentProfilesService using GORM.
type DatabaseAgentProfilesService struct {
	db     *gorm.DB
	logger *slog.Logger
}

// Ensure DatabaseAgentProfilesService implements AgentProfilesService.
var _ AgentProfilesService = (*DatabaseAgentProfilesService)(nil)

// NewDatabaseAgentProfilesService creates an AgentProfilesService backed by a database.
// tablePrefix is applied as GORM NamingStrategy.TablePrefix; empty means no prefix.
func NewDatabaseAgentProfilesService(
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (*DatabaseAgentProfilesService, error) {
	cfg := gormsonal.NewGormConfigForSonalmodTables(gormsonal.GormSonalmodTablesOpts{
		TablePrefix:    tablePrefix,
		TranslateError: true,
	})
	db, err := gorm.Open(gormsonal.NewGormDialector(dsn), cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &DatabaseAgentProfilesService{
		db:     db,
		logger: logger,
	}, nil
}

// AutoMigrate creates or updates the agent_profiles table schema.
func (s *DatabaseAgentProfilesService) AutoMigrate() error {
	return s.db.AutoMigrate(&agentProfileModel{})
}

func (s *DatabaseAgentProfilesService) List(_ context.Context) ([]AgentProfile, error) {
	var models []agentProfileModel
	if err := s.db.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list agent profiles: %w", err)
	}

	profiles := make([]AgentProfile, 0, len(models))
	for _, model := range models {
		profiles = append(profiles, dbModelToProfile(model))
	}

	return profiles, nil
}

func (s *DatabaseAgentProfilesService) Get(_ context.Context, name string) (*AgentProfile, error) {
	var model agentProfileModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
		}
		return nil, fmt.Errorf("get agent profile: %w", err)
	}

	profile := dbModelToProfile(model)
	return &profile, nil
}

func (s *DatabaseAgentProfilesService) Create(
	_ context.Context,
	params CreateAgentProfileParams,
) (*AgentProfile, error) {
	normalized, err := normalizeCreateParams(params)
	if err != nil {
		return nil, err
	}

	model := agentProfileModel{
		Name:              normalized.Name,
		DisplayName:       normalized.DisplayName,
		Role:              normalized.Role,
		Instructions:      normalized.Instructions,
		ToolRefs:          normalized.ToolRefs,
		ExecutionSettings: executionSettingsToDBModel(normalized.ExecutionSettings),
	}
	if err = s.db.Create(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, fmt.Errorf("%w: %s", ErrAgentProfileNameConflict, normalized.Name)
		}
		return nil, fmt.Errorf("create agent profile: %w", err)
	}

	profile := dbModelToProfile(model)
	return &profile, nil
}

func (s *DatabaseAgentProfilesService) Update(
	_ context.Context,
	name string,
	params UpdateAgentProfileParams,
) (*AgentProfile, error) {
	var model agentProfileModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
		}
		return nil, fmt.Errorf("get agent profile for update: %w", err)
	}

	existing := dbModelToProfile(model)
	updated, err := applyProfileUpdate(existing, params)
	if err != nil {
		return nil, err
	}

	model.DisplayName = updated.DisplayName
	model.Role = updated.Role
	model.Instructions = updated.Instructions
	model.ToolRefs = updated.ToolRefs
	model.ExecutionSettings = executionSettingsToDBModel(updated.ExecutionSettings)

	if err = s.db.Save(&model).Error; err != nil {
		return nil, fmt.Errorf("update agent profile: %w", err)
	}

	profile := dbModelToProfile(model)
	return &profile, nil
}

func (s *DatabaseAgentProfilesService) Delete(_ context.Context, name string) error {
	result := s.db.Where("name = ?", name).Delete(&agentProfileModel{})
	if result.Error != nil {
		return fmt.Errorf("delete agent profile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrAgentProfileNotFound, name)
	}

	return nil
}

func dbModelToProfile(model agentProfileModel) AgentProfile {
	return AgentProfile{
		Name:              model.Name,
		DisplayName:       model.DisplayName,
		Role:              model.Role,
		Instructions:      model.Instructions,
		ToolRefs:          model.ToolRefs,
		ExecutionSettings: dbExecutionSettingsToDomain(model.ExecutionSettings),
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
	}
}

func executionSettingsToDBModel(settings ExecutionSettings) dbExecutionSettings {
	model := dbExecutionSettings{
		Mode:         settings.Mode,
		DefaultModel: settings.DefaultModel,
		Cwd:          settings.Cwd,
	}
	if settings.ModeOrDefault() == ExecutionModeACPStdio {
		model.AgentCommand = &dbACPStdioAgentCommand{
			Command: settings.AgentCommand.Command,
			Args:    append([]string(nil), settings.AgentCommand.Args...),
		}
	}

	return model
}

func dbExecutionSettingsToDomain(model dbExecutionSettings) ExecutionSettings {
	settings := ExecutionSettings{
		Mode:         model.Mode,
		DefaultModel: model.DefaultModel,
		Cwd:          model.Cwd,
	}
	if model.AgentCommand != nil {
		settings.AgentCommand = ACPStdioAgentCommand{
			Command: model.AgentCommand.Command,
			Args:    append([]string(nil), model.AgentCommand.Args...),
		}
	}

	return settings
}
