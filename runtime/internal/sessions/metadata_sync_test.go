package sessions_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/sessions"
	"github.com/gemyago/sonalmod/runtime/internal/summarize"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// testPairSessionsStorage adapts [session.Service] and [sessions.SessionMetadataStore] to [sessions.SessionsStorage] for tests.
type testPairSessionsStorage struct {
	session.Service

	meta sessions.SessionMetadataStore
}

func (s *testPairSessionsStorage) SaveMetadata(ctx context.Context, m sessions.SessionMetadata) error {
	return s.meta.Save(ctx, m)
}

func (s *testPairSessionsStorage) ListMetadata(
	ctx context.Context,
	p sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	return s.meta.List(ctx, p)
}

func (s *testPairSessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
	return s.meta.Delete(ctx, appName, userID, sessionID)
}

func (s *testPairSessionsStorage) AutoMigrate() error {
	return nil
}

var _ sessions.SessionsStorage = (*testPairSessionsStorage)(nil)

func TestMetadataSyncStorage(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))

	t.Run("Create delegates and saves metadata via upsert", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(inner, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word() + fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		require.NotNil(t, cr.Session)

		sid := cr.Session.ID()
		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Equal(t, sid, res.Sessions[0].SessionID)
		require.Equal(t, app, res.Sessions[0].AppName)
		require.Equal(t, user, res.Sessions[0].UserID)
		require.Empty(t, res.Sessions[0].Title)
	})

	t.Run("AppendEvent delegates and upserts metadata updatedAt", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(inner, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sess := cr.Session

		firstList, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		u0 := firstList.Sessions[0].UpdatedAt

		time.Sleep(20 * time.Millisecond)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleModel),
					Parts: []*genai.Part{{Text: fake.Lorem().Word()}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), sess, ev))

		secondList, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.True(t, secondList.Sessions[0].UpdatedAt.After(u0),
			"expected updatedAt to advance after AppendEvent")
	})

	t.Run("AppendEvent calls summarizer for first user message when title is empty", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()

		wantTitle := fake.Lorem().Sentence(3)
		var sawText string
		sum := summarizerFunc(func(ctx context.Context, text string) (string, error) {
			_ = ctx
			sawText = text
			return wantTitle, nil
		})
		dec := sessions.NewMetadataSyncStorage(inner, sum, logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		msg := fake.Lorem().Sentence(8)
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: msg}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))
		assert.Equal(t, msg, sawText)

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, wantTitle, res.Sessions[0].Title)
	})

	t.Run("AppendEvent does not overwrite existing title", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()

		existingTitle := fake.Lorem().Sentence(4)
		sum := summarizerFunc(func(context.Context, string) (string, error) {
			return "should-not-appear", nil
		})
		dec := sessions.NewMetadataSyncStorage(inner, sum, logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sid := cr.Session.ID()

		require.NoError(t, inner.SaveMetadata(t.Context(), sessions.SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     existingTitle,
			CreatedAt: time.Now().UTC().Truncate(time.Second),
			UpdatedAt: time.Now().UTC().Truncate(time.Second),
		}))

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Paragraph(2)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, existingTitle, res.Sessions[0].Title)
	})

	t.Run("AppendEvent with non-user role does not set title", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()

		called := false
		sum := summarizerFunc(func(context.Context, string) (string, error) {
			called = true
			return fake.Lorem().Word(), nil
		})
		dec := sessions.NewMetadataSyncStorage(inner, sum, logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleModel),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(4)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))
		require.False(t, called)

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Empty(t, res.Sessions[0].Title)
	})

	t.Run("Delete delegates and deletes metadata", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(inner, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sid := cr.Session.ID()

		require.NoError(t, dec.Delete(t.Context(), &session.DeleteRequest{
			AppName: app, UserID: user, SessionID: sid,
		}))

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, res.Total)
	})

	t.Run("metadata save failure does not fail Create", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		realStore, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		store := &saveFailMetadataStore{inner: realStore, failRemaining: 1}
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    store,
		}
		dec := sessions.NewMetadataSyncStorage(paired, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		require.NotNil(t, cr.Session)
	})

	t.Run("metadata save failure does not fail AppendEvent", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		realStore, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		store := &saveFailMetadataStore{inner: realStore, failRemaining: 1}
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    store,
		}
		dec := sessions.NewMetadataSyncStorage(paired, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))
	})

	t.Run("AppendEvent upserts metadata when Create-time save failed", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		realStore, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		store := &saveFailMetadataStore{inner: realStore, failRemaining: 1}
		wantTitle := fake.Lorem().Word() + "-title"
		sum := summarizerFunc(func(context.Context, string) (string, error) {
			return wantTitle, nil
		})
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    store,
		}
		dec := sessions.NewMetadataSyncStorage(paired, sum, logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sid := cr.Session.ID()

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(5)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Equal(t, sid, res.Sessions[0].SessionID)
		require.Equal(t, wantTitle, res.Sessions[0].Title)
	})

	t.Run("Get and List delegate to inner", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(inner, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sid := cr.Session.ID()

		got, err := dec.Get(t.Context(), &session.GetRequest{
			AppName: app, UserID: user, SessionID: sid,
		})
		require.NoError(t, err)
		require.Equal(t, sid, got.Session.ID())

		lr, err := dec.List(t.Context(), &session.ListRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		require.Len(t, lr.Sessions, 1)
	})

	t.Run("Create skips metadata save when inner returns nil session", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		mem := session.InMemoryService()
		base := t.TempDir()
		store, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		stub := &stubSessionService{
			inner: mem,
			createFn: func(context.Context, *session.CreateRequest) (*session.CreateResponse, error) {
				return &session.CreateResponse{Session: nil}, nil
			},
		}
		paired := &testPairSessionsStorage{Service: stub, meta: store}
		dec := sessions.NewMetadataSyncStorage(paired, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		_, err = dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, res.Total)
	})

	t.Run("Delete returns inner error without metadata delete", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		mem := session.InMemoryService()
		base := t.TempDir()
		store, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		stub := &stubSessionService{
			inner: mem,
			deleteFn: func(context.Context, *session.DeleteRequest) error {
				return errors.New("inner delete failed")
			},
		}
		paired := &testPairSessionsStorage{Service: stub, meta: store}
		dec := sessions.NewMetadataSyncStorage(paired, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		err = dec.Delete(t.Context(), &session.DeleteRequest{
			AppName: app, UserID: user, SessionID: cr.Session.ID(),
		})
		require.Error(t, err)

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
	})

	t.Run("AppendEvent continues when listing metadata fails", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		logBuf := &bytes.Buffer{}
		store := &errListMetadataStore{err: errors.New("list failed")}
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    store,
		}
		dec := sessions.NewMetadataSyncStorage(
			paired,
			summarize.NewTruncatingSummarizer(),
			slog.New(slog.NewTextHandler(logBuf, nil)),
		)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))
		require.Contains(t, logBuf.String(), "list for append")
	})

	t.Run("AppendEvent logs summarizer error and keeps title empty", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		sum := summarizerFunc(func(context.Context, string) (string, error) {
			return "", errors.New("summarizer failed")
		})
		logBuf := &bytes.Buffer{}
		dec := sessions.NewMetadataSyncStorage(inner, sum, slog.New(slog.NewTextHandler(logBuf, nil)))

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(4)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))

		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.Empty(t, res.Sessions[0].Title)
		require.Contains(t, logBuf.String(), "summarize title")
	})

	t.Run("findSessionMetadata paginates across list pages", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		adk := session.InMemoryService()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()

		cr, err := adk.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		targetID := cr.Session.ID()

		base := time.Now().UTC().Truncate(time.Second)
		var all []sessions.SessionMetadata
		for i := range 100 {
			all = append(all, sessions.SessionMetadata{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				UpdatedAt: base.Add(time.Duration(i+1) * time.Minute),
			})
		}
		all = append(all, sessions.SessionMetadata{
			SessionID: targetID,
			AppName:   app,
			UserID:    user,
			Title:     "",
			CreatedAt: base,
			UpdatedAt: base,
		})

		store := &pagedSliceMetadataStore{all: all}
		paired := &testPairSessionsStorage{Service: adk, meta: store}
		dec := sessions.NewMetadataSyncStorage(paired, summarize.NewTruncatingSummarizer(), logger)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))

		page2, err := store.List(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 100, Offset: 100,
		})
		require.NoError(t, err)
		require.Len(t, page2.Sessions, 1)
		require.Equal(t, targetID, page2.Sessions[0].SessionID)
		require.NotEmpty(t, page2.Sessions[0].Title)
	})

	t.Run("AppendEvent skips metadata update for partial events", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		inner := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(inner, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		before, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		u0 := before.Sessions[0].UpdatedAt

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Partial: true,
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))

		after, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app, UserID: user, Limit: 10, Offset: 0,
		})
		require.NoError(t, err)
		require.True(t, u0.Equal(after.Sessions[0].UpdatedAt))
	})

	t.Run("AppendEvent logs metadata save error", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		realStore, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		logBuf := &bytes.Buffer{}
		store := &failOnNthSaveStore{inner: realStore, failOn: 2}
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    store,
		}
		dec := sessions.NewMetadataSyncStorage(
			paired,
			summarize.NewTruncatingSummarizer(),
			slog.New(slog.NewTextHandler(logBuf, nil)),
		)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		ev := &session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Role:  string(genai.RoleUser),
					Parts: []*genai.Part{{Text: fake.Lorem().Sentence(3)}},
				},
			},
			Timestamp: time.Now(),
		}
		require.NoError(t, dec.AppendEvent(t.Context(), cr.Session, ev))
		require.Contains(t, logBuf.String(), "save after append")
	})

	t.Run("Delete logs metadata error but returns nil", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		realStore, err := sessions.NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		logBuf := &bytes.Buffer{}
		paired := &testPairSessionsStorage{
			Service: session.InMemoryService(),
			meta:    &errDeleteMetadataStore{inner: realStore},
		}
		dec := sessions.NewMetadataSyncStorage(
			paired,
			summarize.NewTruncatingSummarizer(),
			slog.New(slog.NewTextHandler(logBuf, nil)),
		)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)

		require.NoError(t, dec.Delete(t.Context(), &session.DeleteRequest{
			AppName: app, UserID: user, SessionID: cr.Session.ID(),
		}))
		require.Contains(t, logBuf.String(), "session metadata: delete")
	})
}

func TestNewMetadataSyncStorageContract(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 1}))

	t.Run("returns sync wrapper and Create syncs ListMetadata", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		mem := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(mem, summarize.NewTruncatingSummarizer(), logger)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		res, err := dec.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Equal(t, cr.Session.ID(), res.Sessions[0].SessionID)
	})

	t.Run("SaveMetadata DeleteMetadata pass through to inner", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		mem := sessions.NewMemorySessionsStorage()
		dec := sessions.NewMetadataSyncStorage(mem, summarize.NewTruncatingSummarizer(), logger)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		cr, err := dec.Create(t.Context(), &session.CreateRequest{AppName: app, UserID: user})
		require.NoError(t, err)
		sid := cr.Session.ID()

		require.NoError(t, dec.DeleteMetadata(t.Context(), app, user, sid))
		res, err := mem.ListMetadata(t.Context(), sessions.ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
		})
		require.NoError(t, err)
		require.Equal(t, 0, res.Total)
	})

	t.Run("AutoMigrate delegates to inner storage", func(t *testing.T) {
		t.Parallel()
		inner := sessions.NewMemorySessionsStorage()
		var calls int
		wrapped := &sessionsStorageAutoMigrateSpy{
			SessionsStorage: inner,
			onAutoMigrate: func() error {
				calls++
				return inner.AutoMigrate()
			},
		}
		dec := sessions.NewMetadataSyncStorage(wrapped, summarize.NewTruncatingSummarizer(), logger)
		require.NoError(t, dec.AutoMigrate())
		require.Equal(t, 1, calls)
	})
}

// sessionsStorageAutoMigrateSpy wraps [sessions.SessionsStorage] to count [AutoMigrate] calls.
type sessionsStorageAutoMigrateSpy struct {
	sessions.SessionsStorage

	onAutoMigrate func() error
}

func (s *sessionsStorageAutoMigrateSpy) AutoMigrate() error {
	return s.onAutoMigrate()
}

var _ sessions.SessionsStorage = (*sessionsStorageAutoMigrateSpy)(nil)

type summarizerFunc func(ctx context.Context, text string) (string, error)

func (f summarizerFunc) Summarize(ctx context.Context, text string) (string, error) {
	return f(ctx, text)
}

type saveFailMetadataStore struct {
	inner         sessions.SessionMetadataStore
	failRemaining int
}

func (s *saveFailMetadataStore) Save(ctx context.Context, metadata sessions.SessionMetadata) error {
	if s.failRemaining > 0 {
		s.failRemaining--
		return errors.New("injected save failure")
	}
	return s.inner.Save(ctx, metadata)
}

func (s *saveFailMetadataStore) List(
	ctx context.Context,
	params sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	return s.inner.List(ctx, params)
}

func (s *saveFailMetadataStore) Delete(ctx context.Context, appName, userID, sessionID string) error {
	return s.inner.Delete(ctx, appName, userID, sessionID)
}

type stubSessionService struct {
	inner    session.Service
	createFn func(context.Context, *session.CreateRequest) (*session.CreateResponse, error)
	deleteFn func(context.Context, *session.DeleteRequest) error
}

func (s *stubSessionService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if s.createFn != nil {
		return s.createFn(ctx, req)
	}
	return s.inner.Create(ctx, req)
}

func (s *stubSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	return s.inner.Get(ctx, req)
}

func (s *stubSessionService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	return s.inner.List(ctx, req)
}

func (s *stubSessionService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, req)
	}
	return s.inner.Delete(ctx, req)
}

func (s *stubSessionService) AppendEvent(ctx context.Context, sess session.Session, event *session.Event) error {
	return s.inner.AppendEvent(ctx, sess, event)
}

type errListMetadataStore struct {
	err error
}

func (e *errListMetadataStore) Save(context.Context, sessions.SessionMetadata) error {
	return nil
}

func (e *errListMetadataStore) List(
	_ context.Context,
	_ sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	return nil, e.err
}

func (e *errListMetadataStore) Delete(context.Context, string, string, string) error {
	return nil
}

type errDeleteMetadataStore struct {
	inner sessions.SessionMetadataStore
}

func (e *errDeleteMetadataStore) Save(ctx context.Context, m sessions.SessionMetadata) error {
	return e.inner.Save(ctx, m)
}

func (e *errDeleteMetadataStore) List(
	ctx context.Context,
	p sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	return e.inner.List(ctx, p)
}

func (e *errDeleteMetadataStore) Delete(context.Context, string, string, string) error {
	return errors.New("metadata delete failed")
}

type failOnNthSaveStore struct {
	inner  sessions.SessionMetadataStore
	failOn int
	n      int
}

func (f *failOnNthSaveStore) Save(ctx context.Context, m sessions.SessionMetadata) error {
	f.n++
	if f.n == f.failOn {
		return errors.New("injected save failure")
	}
	return f.inner.Save(ctx, m)
}

func (f *failOnNthSaveStore) List(
	ctx context.Context,
	p sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	return f.inner.List(ctx, p)
}

func (f *failOnNthSaveStore) Delete(ctx context.Context, appName, userID, sessionID string) error {
	return f.inner.Delete(ctx, appName, userID, sessionID)
}

// pagedSliceMetadataStore is an in-memory [sessions.SessionMetadataStore] used to exercise paginated
// lookup without relying on wall-clock ordering of many creates.
type pagedSliceMetadataStore struct {
	mu  sync.Mutex
	all []sessions.SessionMetadata
}

func (s *pagedSliceMetadataStore) Save(ctx context.Context, m sessions.SessionMetadata) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.all {
		if s.all[i].SessionID == m.SessionID {
			s.all[i] = m
			return nil
		}
	}
	s.all = append(s.all, m)
	return nil
}

func (s *pagedSliceMetadataStore) List(
	ctx context.Context,
	p sessions.ListSessionMetadataParams,
) (*sessions.ListSessionMetadataResult, error) {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	var matched []sessions.SessionMetadata
	for _, m := range s.all {
		if m.AppName == p.AppName && m.UserID == p.UserID {
			matched = append(matched, m)
		}
	}
	slices.SortFunc(matched, func(a, b sessions.SessionMetadata) int {
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return 1
		}
		return 0
	})
	total := len(matched)
	start := min(p.Offset, total)
	end := min(start+p.Limit, total)
	return &sessions.ListSessionMetadataResult{
		Sessions: matched[start:end],
		Total:    total,
	}, nil
}

func (s *pagedSliceMetadataStore) Delete(ctx context.Context, appName, userID, sessionID string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.all[:0]
	for _, m := range s.all {
		if m.SessionID == sessionID && m.AppName == appName && m.UserID == userID {
			continue
		}
		out = append(out, m)
	}
	s.all = out
	return nil
}
