package gormsonal

import (
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// GormSonalmodTablesOpts configures GORM for Sonalmod-managed relational tables.
type GormSonalmodTablesOpts struct {
	// TablePrefix is passed to NamingStrategy.TablePrefix. Empty means no prefix.
	TablePrefix string
	// TranslateError enables GORM dialect error translation (for example ErrRecordNotFound).
	TranslateError bool
}

// NewGormConfigForSonalmodTables returns a shared GORM config for session storage, provider config,
// and other Sonalmod database-backed services so physical table names use the same prefix.
func NewGormConfigForSonalmodTables(opts GormSonalmodTablesOpts) *gorm.Config {
	return &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: opts.TablePrefix},
		TranslateError: opts.TranslateError,
	}
}
