package llmproviders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/gormsonal"
	"gorm.io/gorm"
)

// providerModelConfig is the JSON-serializable model config entry stored in the DB.
type providerModelConfig struct {
	Name          string `json:"name"`
	DisplayName   string `json:"displayName"`
	Summarization bool   `json:"summarization"`
}

// providerConfigModel is the GORM model for a provider config record.
type providerConfigModel struct {
	Name        string                `gorm:"column:name;primaryKey;size:255"`
	Type        string                `gorm:"column:type;size:50;not null"`
	DisplayName string                `gorm:"column:display_name;size:255"`
	BaseURL     string                `gorm:"column:base_url;size:2048;not null"`
	APIKey      string                `gorm:"column:api_key;size:2048;not null"  json:"-"`
	Models      []providerModelConfig `gorm:"column:models;serializer:json"`
	CreatedAt   time.Time             `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time             `gorm:"column:updated_at;autoUpdateTime"`
}

func (providerConfigModel) TableName() string { return "provider_configs" }

// DatabaseProvidersConfigService implements ProvidersConfigService using GORM.
type DatabaseProvidersConfigService struct {
	db     *gorm.DB
	logger *slog.Logger
}

// Ensure DatabaseProvidersConfigService implements ProvidersConfigService.
var _ ProvidersConfigService = (*DatabaseProvidersConfigService)(nil)

// NewDatabaseProvidersConfigService creates a ProvidersConfigService backed by a database.
// Uses the same DSN detection logic as the session service (SQLite or PostgreSQL).
// tablePrefix is applied as GORM NamingStrategy.TablePrefix; empty means no prefix.
func NewDatabaseProvidersConfigService(
	dsn string,
	logger *slog.Logger,
	tablePrefix string,
) (*DatabaseProvidersConfigService, error) {
	cfg := gormsonal.NewGormConfigForSonalmodTables(gormsonal.GormSonalmodTablesOpts{
		TablePrefix:    tablePrefix,
		TranslateError: true,
	})
	db, err := gorm.Open(gormsonal.NewGormDialector(dsn), cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return &DatabaseProvidersConfigService{
		db:     db,
		logger: logger,
	}, nil
}

// AutoMigrate creates or updates the provider_configs table schema.
func (s *DatabaseProvidersConfigService) AutoMigrate() error {
	return s.db.AutoMigrate(&providerConfigModel{})
}

func (s *DatabaseProvidersConfigService) List(_ context.Context) ([]ProviderConfig, error) {
	var models []providerConfigModel
	if err := s.db.Order("created_at ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list provider configs: %w", err)
	}
	result := make([]ProviderConfig, 0, len(models))
	for _, m := range models {
		result = append(result, providerModelToConfig(m))
	}
	return result, nil
}

func (s *DatabaseProvidersConfigService) Get(_ context.Context, name string) (*ProviderConfig, error) {
	var model providerConfigModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
		}
		return nil, fmt.Errorf("get provider config: %w", err)
	}
	cfg := providerModelToConfig(model)
	return &cfg, nil
}

func (s *DatabaseProvidersConfigService) Create(
	_ context.Context,
	params CreateProviderConfigParams,
) (*ProviderConfig, error) {
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

	model := providerConfigModel{
		Name:        params.Name,
		Type:        params.Type,
		DisplayName: params.DisplayName,
		BaseURL:     params.BaseURL,
		APIKey:      params.APIKey,
		Models:      modelsToProviderModelConfig(params.Models),
	}
	if err := s.db.Create(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, fmt.Errorf("%w: %s", ErrProviderConfigNameConflict, params.Name)
		}
		return nil, fmt.Errorf("create provider config: %w", err)
	}
	cfg := providerModelToConfig(model)
	return &cfg, nil
}

func (s *DatabaseProvidersConfigService) Update(
	_ context.Context,
	name string,
	params UpdateProviderConfigParams,
) (*ProviderConfig, error) {
	var model providerConfigModel
	if err := s.db.First(&model, "name = ?", name).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
		}
		return nil, fmt.Errorf("get provider config for update: %w", err)
	}

	model.DisplayName = params.DisplayName
	model.BaseURL = params.BaseURL
	if params.APIKey != "" {
		model.APIKey = params.APIKey
	}
	model.Models = modelsToProviderModelConfig(params.Models)

	if err := s.db.Save(&model).Error; err != nil {
		return nil, fmt.Errorf("update provider config: %w", err)
	}
	cfg := providerModelToConfig(model)
	return &cfg, nil
}

func (s *DatabaseProvidersConfigService) Delete(_ context.Context, name string) error {
	result := s.db.Where("name = ?", name).Delete(&providerConfigModel{})
	if result.Error != nil {
		return fmt.Errorf("delete provider config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrProviderConfigNotFound, name)
	}
	return nil
}

func providerModelToConfig(m providerConfigModel) ProviderConfig {
	models := make([]ModelConfig, len(m.Models))
	for i, mc := range m.Models {
		models[i] = ModelConfig(mc)
	}
	return ProviderConfig{
		Name:        m.Name,
		Type:        m.Type,
		DisplayName: m.DisplayName,
		BaseURL:     m.BaseURL,
		APIKey:      m.APIKey,
		Models:      models,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func modelsToProviderModelConfig(models []ModelConfig) []providerModelConfig {
	if models == nil {
		return []providerModelConfig{}
	}
	result := make([]providerModelConfig, len(models))
	for i, m := range models {
		result[i] = providerModelConfig(m)
	}
	return result
}
