package gormsonal

import (
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Dialector wraps a concrete GORM dialector without exposing the interface as a return type.
type Dialector struct {
	gorm.Dialector
}

// isSQLiteDSN returns true if the DSN refers to a SQLite database.
func isSQLiteDSN(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	return dsn == ":memory:" ||
		strings.HasPrefix(dsn, "file:") ||
		strings.Contains(dsn, "sqlite") ||
		strings.HasSuffix(dsn, ".db") ||
		strings.HasSuffix(dsn, ".sqlite")
}

// NewGormDialector returns the appropriate GORM dialector for the given DSN.
// SQLite DSNs (":memory:", "file:...", etc.) use the pure-Go SQLite driver.
// All other DSNs are treated as PostgreSQL.
func NewGormDialector(dsn string) Dialector {
	if isSQLiteDSN(dsn) {
		return Dialector{Dialector: sqlite.Open(dsn)}
	}
	return Dialector{Dialector: postgres.Open(dsn)}
}

// Translate forwards error translation when the wrapped dialector supports it.
func (d Dialector) Translate(err error) error {
	if translator, ok := d.Dialector.(gorm.ErrorTranslator); ok {
		return translator.Translate(err)
	}
	return err
}
