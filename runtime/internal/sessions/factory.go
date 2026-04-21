package sessions

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/gemyago/sonalmod/runtime/internal/gormsonal"
	"github.com/gemyago/sonalmod/runtime/internal/summarize"
)

const (
	sessionStorageTypeFile     = "file"
	sessionStorageTypeDatabase = "database"
	sessionStorageTypeMemory   = "memory"
)

// SessionServiceFactoryDeps configures session storage for both embedder config (SessionStorageType
// string from YAML) and explicit Runner-style flags. UseDatabaseStorage and UseFileStorage are
// mutually exclusive; when both are false, SessionStorageType selects the backend.
type SessionServiceFactoryDeps struct {
	RootLogger *slog.Logger

	UseDatabaseStorage bool
	UseFileStorage     bool

	DatabaseDSN           string
	DatabaseTablePrefix   string
	SessionStorageBaseDir string
	SessionStorageType    string

	// Summarizer produces session titles from the first user message.
	Summarizer summarize.Summarizer
}

// NewSessionsStorage builds listing-metadata sync over the configured backend (file, database, or memory).
func NewSessionsStorage(
	deps SessionServiceFactoryDeps,
) (
	*MetadataSyncStorage,
	error,
) {
	raw, err := newRawSessionsStorage(deps)
	if err != nil {
		return nil, err
	}
	return NewMetadataSyncStorage(raw, deps.Summarizer, deps.RootLogger), nil
}

// newRawSessionsStorage selects the concrete [SessionsStorage] implementation from configuration.
// It returns the interface because the backend type is chosen at runtime (file, database, or memory).
func newRawSessionsStorage( //nolint:ireturn // polymorphic factory; multiple concrete backends
	deps SessionServiceFactoryDeps,
) (
	SessionsStorage,
	error,
) {
	if deps.UseDatabaseStorage {
		return NewDatabaseSessionsStorage(deps.DatabaseDSN, gormsonal.GormSonalmodTablesOpts{
			TablePrefix: deps.DatabaseTablePrefix,
		})
	}
	if deps.UseFileStorage {
		return NewFileSessionsStorage(deps.SessionStorageBaseDir, deps.RootLogger)
	}

	t := strings.TrimSpace(strings.ToLower(deps.SessionStorageType))
	switch t {
	case "", sessionStorageTypeMemory:
		return NewMemorySessionsStorage(), nil
	case sessionStorageTypeDatabase:
		return NewDatabaseSessionsStorage(deps.DatabaseDSN, gormsonal.GormSonalmodTablesOpts{
			TablePrefix: deps.DatabaseTablePrefix,
		})
	case sessionStorageTypeFile:
		return NewFileSessionsStorage(deps.SessionStorageBaseDir, deps.RootLogger)
	default:
		return nil, fmt.Errorf(
			"agentRuntime.storage.type: unsupported value %q (use %q, %q, or %q)",
			t, sessionStorageTypeMemory, sessionStorageTypeFile, sessionStorageTypeDatabase,
		)
	}
}
