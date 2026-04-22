package codinglane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/gormsonal"
	"gorm.io/gorm"
)

type dbOpenCodeAgentCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type dbOpenCodeLaunchOptions struct {
	Transport string `json:"transport"`
}

type openCodeBindingModel struct {
	Name          string                  `gorm:"column:name;primaryKey;size:255"`
	ProfileName   string                  `gorm:"column:profile_name;size:255;not null;index"`
	CWD           string                  `gorm:"column:cwd;size:4096"`
	AgentCommand  dbOpenCodeAgentCommand  `gorm:"column:agent_command;serializer:json"`
	LaunchOptions dbOpenCodeLaunchOptions `gorm:"column:launch_options;serializer:json"`
	CreatedAt     time.Time               `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time               `gorm:"column:updated_at;autoUpdateTime"`
}

func (openCodeBindingModel) TableName() string { return "opencode_bindings" }

// DatabaseOpenCodeBindingService implements OpenCodeBindingService with GORM.
type DatabaseOpenCodeBindingService struct {
	db     *gorm.DB
	logger *slog.Logger
}

// Ensure DatabaseOpenCodeBindingService implements OpenCodeBindingService.
var _ OpenCodeBindingService = (*DatabaseOpenCodeBindingService)(nil)

// NewDatabaseOpenCodeBindingService creates a database-backed OpenCode binding service.
func NewDatabaseOpenCodeBindingService(
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (*DatabaseOpenCodeBindingService, error) {
	cfg := gormsonal.NewGormConfigForSonalmodTables(gormsonal.GormSonalmodTablesOpts{
		TablePrefix:    tablePrefix,
		TranslateError: true,
	})
	db, err := gorm.Open(gormsonal.NewGormDialector(dsn), cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return &DatabaseOpenCodeBindingService{
		db:     db,
		logger: logger,
	}, nil
}

// AutoMigrate creates or updates the opencode_bindings table schema.
func (s *DatabaseOpenCodeBindingService) AutoMigrate() error {
	return s.db.AutoMigrate(&openCodeBindingModel{})
}

func (s *DatabaseOpenCodeBindingService) List(_ context.Context) ([]OpenCodeBinding, error) {
	var models []openCodeBindingModel
	if err := s.db.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list opencode bindings: %w", err)
	}

	bindings := make([]OpenCodeBinding, 0, len(models))
	for _, model := range models {
		bindings = append(bindings, dbModelToBinding(model))
	}
	return bindings, nil
}

func (s *DatabaseOpenCodeBindingService) Get(_ context.Context, name string) (*OpenCodeBinding, error) {
	var model openCodeBindingModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrOpenCodeBindingNotFound, name)
		}
		return nil, fmt.Errorf("get opencode binding: %w", err)
	}

	binding := dbModelToBinding(model)
	return &binding, nil
}

func (s *DatabaseOpenCodeBindingService) Create(
	_ context.Context,
	params CreateOpenCodeBindingParams,
) (*OpenCodeBinding, error) {
	normalized, err := normalizeCreateOpenCodeBindingParams(params)
	if err != nil {
		return nil, err
	}

	model := openCodeBindingModel{
		Name:        normalized.Name,
		ProfileName: normalized.ProfileName,
		CWD:         normalized.CWD,
		AgentCommand: dbOpenCodeAgentCommand{
			Command: normalized.AgentCommand.Command,
			Args:    normalized.AgentCommand.Args,
		},
		LaunchOptions: dbOpenCodeLaunchOptions{
			Transport: normalized.LaunchOptions.Transport,
		},
	}
	if err = s.db.Create(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, fmt.Errorf("%w: %s", ErrOpenCodeBindingNameConflict, normalized.Name)
		}
		return nil, fmt.Errorf("create opencode binding: %w", err)
	}

	binding := dbModelToBinding(model)
	return &binding, nil
}

func (s *DatabaseOpenCodeBindingService) Update(
	_ context.Context,
	name string,
	params UpdateOpenCodeBindingParams,
) (*OpenCodeBinding, error) {
	var model openCodeBindingModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrOpenCodeBindingNotFound, name)
		}
		return nil, fmt.Errorf("get opencode binding for update: %w", err)
	}

	existing := dbModelToBinding(model)
	updated, err := applyOpenCodeBindingUpdate(existing, params)
	if err != nil {
		return nil, err
	}

	model.CWD = updated.CWD
	model.AgentCommand = dbOpenCodeAgentCommand{
		Command: updated.AgentCommand.Command,
		Args:    updated.AgentCommand.Args,
	}
	model.LaunchOptions = dbOpenCodeLaunchOptions{
		Transport: updated.LaunchOptions.Transport,
	}
	if err = s.db.Save(&model).Error; err != nil {
		return nil, fmt.Errorf("update opencode binding: %w", err)
	}

	binding := dbModelToBinding(model)
	return &binding, nil
}

func (s *DatabaseOpenCodeBindingService) Delete(_ context.Context, name string) error {
	result := s.db.Where("name = ?", name).Delete(&openCodeBindingModel{})
	if result.Error != nil {
		return fmt.Errorf("delete opencode binding: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrOpenCodeBindingNotFound, name)
	}
	return nil
}

func dbModelToBinding(model openCodeBindingModel) OpenCodeBinding {
	return OpenCodeBinding{
		Name:        model.Name,
		ProfileName: model.ProfileName,
		CWD:         model.CWD,
		AgentCommand: OpenCodeAgentCommand{
			Command: model.AgentCommand.Command,
			Args:    model.AgentCommand.Args,
		},
		LaunchOptions: OpenCodeLaunchOptions{
			Transport: model.LaunchOptions.Transport,
		},
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
