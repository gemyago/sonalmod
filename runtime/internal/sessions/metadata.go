package sessions

import (
	"context"
	"time"
)

// SessionMetadata holds lightweight fields for listing and indexing sessions.
type SessionMetadata struct {
	SessionID string
	AppName   string
	UserID    string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ListSessionMetadataParams filters and paginates a metadata listing.
type ListSessionMetadataParams struct {
	AppName string
	UserID  string
	Limit   int // required — max number of results to return (1–100)
	Offset  int // optional — number of results to skip (default 0)
}

// ListSessionMetadataResult is a page of session metadata plus total count for pagination.
type ListSessionMetadataResult struct {
	Sessions []SessionMetadata
	Total    int // total count of sessions matching the query (for pagination)
}

// SessionMetadataStore persists lightweight session metadata.
type SessionMetadataStore interface {
	// Save persists metadata using upsert semantics: create if not exists, update if exists.
	Save(ctx context.Context, metadata SessionMetadata) error
	List(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)
	Delete(ctx context.Context, appName, userID, sessionID string) error
}
