package sessions

import (
	"context"
	"errors"
	"slices"
	"sync"

	"google.golang.org/adk/session"
)

// MemorySessionsStorage unifies in-memory ADK session persistence and session metadata.
type MemorySessionsStorage struct {
	session.Service

	meta *MemorySessionMetadataStore
}

// NewMemorySessionsStorage returns concrete *MemorySessionsStorage.
func NewMemorySessionsStorage() *MemorySessionsStorage {
	return &MemorySessionsStorage{
		Service: session.InMemoryService(),
		meta:    NewMemorySessionMetadataStore(),
	}
}

func (s *MemorySessionsStorage) SaveMetadata(ctx context.Context, m SessionMetadata) error {
	return s.meta.Save(ctx, m)
}

func (s *MemorySessionsStorage) ListMetadata(
	ctx context.Context,
	p ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return s.meta.List(ctx, p)
}

func (s *MemorySessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
	return s.meta.Delete(ctx, appName, userID, sessionID)
}

func (s *MemorySessionsStorage) AutoMigrate() error {
	return nil
}

var _ SessionsStorage = (*MemorySessionsStorage)(nil)
var _ session.Service = (*MemorySessionsStorage)(nil)

// MemorySessionMetadataStore is an in-process [SessionMetadataStore] for use when ADK session
// storage is in-memory (no durable session path). Metadata is lost when the process exits.
type MemorySessionMetadataStore struct {
	mu   sync.RWMutex
	data []SessionMetadata
}

var _ SessionMetadataStore = (*MemorySessionMetadataStore)(nil)

// NewMemorySessionMetadataStore returns a new empty in-memory metadata store.
func NewMemorySessionMetadataStore() *MemorySessionMetadataStore {
	return &MemorySessionMetadataStore{}
}

// Save upserts metadata by session ID (same app/user/session key).
func (s *MemorySessionMetadataStore) Save(ctx context.Context, metadata SessionMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateMetadataForSave(metadata); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data {
		if s.data[i].SessionID == metadata.SessionID &&
			s.data[i].AppName == metadata.AppName &&
			s.data[i].UserID == metadata.UserID {
			s.data[i] = metadata
			return nil
		}
	}
	s.data = append(s.data, metadata)
	return nil
}

// List returns a page of metadata for the app/user, newest first.
func (s *MemorySessionMetadataStore) List(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateListParams(params); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var matched []SessionMetadata
	for _, m := range s.data {
		if m.AppName == params.AppName && m.UserID == params.UserID {
			matched = append(matched, m)
		}
	}
	sortByUpdatedAtDesc(matched)
	total := len(matched)
	start := min(params.Offset, total)
	end := min(start+params.Limit, total)
	page := matched[start:end]
	out := make([]SessionMetadata, len(page))
	copy(out, page)
	return &ListSessionMetadataResult{Sessions: out, Total: total}, nil
}

// Delete removes metadata for the session if present.
func (s *MemorySessionMetadataStore) Delete(ctx context.Context, appName, userID, sessionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if appName == "" || userID == "" {
		return errors.New("app_name and user_id are required")
	}
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.data[:0]
	for _, m := range s.data {
		if m.SessionID == sessionID && m.AppName == appName && m.UserID == userID {
			continue
		}
		out = append(out, m)
	}
	s.data = out
	return nil
}

// ValidateMetadataForSave returns an error if required metadata fields are missing for persistence.
func ValidateMetadataForSave(m SessionMetadata) error {
	if m.SessionID == "" {
		return errors.New("session_id is required")
	}
	if m.AppName == "" {
		return errors.New("app_name is required")
	}
	if m.UserID == "" {
		return errors.New("user_id is required")
	}
	return nil
}

// ValidateListParams returns an error if listing parameters are invalid.
func ValidateListParams(p ListSessionMetadataParams) error {
	if p.AppName == "" || p.UserID == "" {
		return errors.New("app_name and user_id are required")
	}
	if p.Limit < 1 || p.Limit > 100 {
		return errors.New("limit must be between 1 and 100")
	}
	if p.Offset < 0 {
		return errors.New("offset must be non-negative")
	}
	return nil
}

func sortByUpdatedAtDesc(sessions []SessionMetadata) {
	slices.SortFunc(sessions, func(a, b SessionMetadata) int {
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return 1
		}
		return 0
	})
}
