package sessions

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/summarize"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// sessionMetadataListPageSize is the page size used when scanning metadata for a session by ID.
// It matches the maximum allowed [ListSessionMetadataParams.Limit] value.
const sessionMetadataListPageSize = 100

// MetadataSyncStorage wraps [SessionsStorage] to keep session metadata in sync on
// Create, AppendEvent, and Delete. Get and List delegate to the inner storage unchanged.
type MetadataSyncStorage struct {
	inner      SessionsStorage
	summarizer summarize.Summarizer
	logger     *slog.Logger
}

var _ SessionsStorage = (*MetadataSyncStorage)(nil)

// NewMetadataSyncStorage wraps inner and updates listing metadata (and titles via summarizer)
// on Create, AppendEvent, and Delete. Metadata failures are logged and do not fail the primary operation.
func NewMetadataSyncStorage(
	inner SessionsStorage,
	summarizer summarize.Summarizer,
	logger *slog.Logger,
) *MetadataSyncStorage {
	return &MetadataSyncStorage{
		inner:      inner,
		summarizer: summarizer,
		logger:     logger,
	}
}

func (m *MetadataSyncStorage) SaveMetadata(ctx context.Context, meta SessionMetadata) error {
	return m.inner.SaveMetadata(ctx, meta)
}

func (m *MetadataSyncStorage) ListMetadata(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return m.inner.ListMetadata(ctx, params)
}

func (m *MetadataSyncStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
	return m.inner.DeleteMetadata(ctx, appName, userID, sessionID)
}

func (m *MetadataSyncStorage) AutoMigrate() error {
	return m.inner.AutoMigrate()
}

func (m *MetadataSyncStorage) Create(
	ctx context.Context,
	req *session.CreateRequest,
) (*session.CreateResponse, error) {
	resp, err := m.inner.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Session == nil {
		return resp, nil
	}
	now := time.Now()
	meta := SessionMetadata{
		SessionID: resp.Session.ID(),
		AppName:   resp.Session.AppName(),
		UserID:    resp.Session.UserID(),
		Title:     "",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if saveErr := m.inner.SaveMetadata(ctx, meta); saveErr != nil {
		m.logError(ctx, "session metadata: save after create", saveErr, "session_id", meta.SessionID)
	}
	return resp, nil
}

func (m *MetadataSyncStorage) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	return m.inner.Get(ctx, req)
}

func (m *MetadataSyncStorage) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	return m.inner.List(ctx, req)
}

func (m *MetadataSyncStorage) Delete(ctx context.Context, req *session.DeleteRequest) error {
	if err := m.inner.Delete(ctx, req); err != nil {
		return err
	}
	if delErr := m.inner.DeleteMetadata(ctx, req.AppName, req.UserID, req.SessionID); delErr != nil {
		m.logError(ctx, "session metadata: delete", delErr, "session_id", req.SessionID)
	}
	return nil
}

func (m *MetadataSyncStorage) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	if err := m.inner.AppendEvent(ctx, sess, event); err != nil {
		return err
	}
	if event == nil || event.Partial {
		return nil
	}
	if sess == nil {
		return nil
	}

	appName := sess.AppName()
	userID := sess.UserID()
	sessionID := sess.ID()

	existing, found, findErr := m.findSessionMetadata(ctx, appName, userID, sessionID)
	if findErr != nil {
		m.logError(ctx, "session metadata: list for append", findErr, "session_id", sessionID)
		return nil
	}

	now := time.Now()
	createdAt := now
	if found {
		createdAt = existing.CreatedAt
	}
	title := m.titleAfterAppend(ctx, found, existing, sessionID, event)

	meta := SessionMetadata{
		SessionID: sessionID,
		AppName:   appName,
		UserID:    userID,
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	if saveErr := m.inner.SaveMetadata(ctx, meta); saveErr != nil {
		m.logError(ctx, "session metadata: save after append", saveErr, "session_id", sessionID)
	}
	return nil
}

func (m *MetadataSyncStorage) titleAfterAppend(
	ctx context.Context,
	found bool,
	existing SessionMetadata,
	sessionID string,
	event *session.Event,
) string {
	if found && existing.Title != "" {
		return existing.Title
	}
	txt := userMessageText(event)
	if txt == "" {
		return ""
	}
	t2, sumErr := m.summarizer.Summarize(ctx, txt)
	if sumErr != nil {
		m.logError(ctx, "session metadata: summarize title", sumErr, "session_id", sessionID)
		return ""
	}
	return t2
}

func (m *MetadataSyncStorage) findSessionMetadata(
	ctx context.Context,
	appName, userID, sessionID string,
) (SessionMetadata, bool, error) {
	offset := 0
	for {
		res, err := m.inner.ListMetadata(ctx, ListSessionMetadataParams{
			AppName: appName,
			UserID:  userID,
			Limit:   sessionMetadataListPageSize,
			Offset:  offset,
		})
		if err != nil {
			return SessionMetadata{}, false, err
		}
		for _, meta := range res.Sessions {
			if meta.SessionID == sessionID {
				return meta, true, nil
			}
		}
		if offset+len(res.Sessions) >= res.Total {
			return SessionMetadata{}, false, nil
		}
		offset += len(res.Sessions)
	}
}

func userMessageText(ev *session.Event) string {
	if ev == nil || ev.Content == nil {
		return ""
	}
	if ev.Content.Role != string(genai.RoleUser) {
		return ""
	}
	var b strings.Builder
	for _, p := range ev.Content.Parts {
		if p != nil && p.Text != "" {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

func (m *MetadataSyncStorage) logError(ctx context.Context, msg string, err error, args ...any) {
	if m.logger == nil {
		return
	}
	m.logger.ErrorContext(ctx, msg, append([]any{"err", err}, args...)...)
}
