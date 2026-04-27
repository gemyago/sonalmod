package gormsonal

import (
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

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
//
//nolint:ireturn // GORM expects its Dialector interface here.
func NewGormDialector(dsn string) gorm.Dialector {
	if isSQLiteDSN(dsn) {
		return sqlite.Open(dsn)
	}
	return postgres.Open(dsn)
}
