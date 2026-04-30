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
	var (
		raw SessionsStorage
		err error
	)
	if deps.UseDatabaseStorage {
		raw, err = NewDatabaseSessionsStorage(deps.DatabaseDSN, gormsonal.GormSonalmodTablesOpts{
			TablePrefix: deps.DatabaseTablePrefix,
		})
		if err != nil {
			return nil, err
		}
		return NewMetadataSyncStorage(raw, deps.Summarizer, deps.RootLogger), nil
	}
	if deps.UseFileStorage {
		raw, err = NewFileSessionsStorage(deps.SessionStorageBaseDir, deps.RootLogger)
		if err != nil {
			return nil, err
		}
		return NewMetadataSyncStorage(raw, deps.Summarizer, deps.RootLogger), nil
	}

	switch t := strings.TrimSpace(strings.ToLower(deps.SessionStorageType)); t {
	case "", sessionStorageTypeMemory:
		raw = NewMemorySessionsStorage()
	case sessionStorageTypeDatabase:
		raw, err = NewDatabaseSessionsStorage(deps.DatabaseDSN, gormsonal.GormSonalmodTablesOpts{
			TablePrefix: deps.DatabaseTablePrefix,
		})
	case sessionStorageTypeFile:
		raw, err = NewFileSessionsStorage(deps.SessionStorageBaseDir, deps.RootLogger)
	default:
		return nil, fmt.Errorf(
			"agentRuntime.storage.type: unsupported value %q (use %q, %q, or %q)",
			t, sessionStorageTypeMemory, sessionStorageTypeFile, sessionStorageTypeDatabase,
		)
	}

	if err != nil {
		return nil, err
	}

	return NewMetadataSyncStorage(raw, deps.Summarizer, deps.RootLogger), nil
}
