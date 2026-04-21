package sessions

import (
	"context"

	"google.golang.org/adk/session"
)

// SessionsStorage is the unified session persistence component.
// It extends the ADK session service with session metadata operations and database migration.
// All implementations are in the same module; this interface is the only type consumers depend on.
//
//nolint:revive // Name is intentional: distinguishes this contract from ADK session.Service ("session" stutter is acceptable here).
type SessionsStorage interface {
	session.Service // embed: Create, Get, List, Delete, AppendEvent

	// SaveMetadata persists lightweight session metadata (upsert semantics).
	SaveMetadata(ctx context.Context, metadata SessionMetadata) error

	// ListMetadata returns a paginated list of session metadata.
	ListMetadata(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)

	// DeleteMetadata removes session metadata for the given session.
	DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error

	// AutoMigrate runs database schema migrations. No-op for file and in-memory backends.
	AutoMigrate() error
}

// AutoMigratable is implemented by services that support database schema migration.
type AutoMigratable interface {
	AutoMigrate() error
}
