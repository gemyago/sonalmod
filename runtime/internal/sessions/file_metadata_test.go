package sessions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
)

func TestFileSessionMetadataStore(t *testing.T) {
	t.Parallel()

	t.Run("Save creates new metadata entry", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word() + fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		created := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     fake.Lorem().Sentence(4),
			CreatedAt: created,
			UpdatedAt: created,
		}
		require.NoError(t, store.Save(t.Context(), meta))

		idx := sessionMetadataIndexPath(base, app, user)
		_, statErr := os.Stat(idx)
		require.NoError(t, statErr)

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Len(t, res.Sessions, 1)
		require.Equal(t, meta, res.Sessions[0])
	})

	t.Run("Save updates existing metadata entry (upsert)", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		t0 := time.Now().UTC().Truncate(time.Second)
		first := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "first-title",
			CreatedAt: t0,
			UpdatedAt: t0,
		}
		require.NoError(t, store.Save(t.Context(), first))

		t1 := t0.Add(time.Hour)
		second := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "replaced-title",
			CreatedAt: t0,
			UpdatedAt: t1,
		}
		require.NoError(t, store.Save(t.Context(), second))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Equal(t, second, res.Sessions[0])
	})

	t.Run("Save creates entry when index file did not exist (upsert recovery)", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "",
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, store.Save(t.Context(), meta))

		idx := sessionMetadataIndexPath(base, app, user)
		require.FileExists(t, idx)
	})

	t.Run("List returns entries sorted by updatedAt desc", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		baseTime := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)

		olderID := fake.UUID().V4()
		newerID := fake.UUID().V4()
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: olderID,
			AppName:   app,
			UserID:    user,
			Title:     "old",
			CreatedAt: baseTime,
			UpdatedAt: baseTime,
		}))
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: newerID,
			AppName:   app,
			UserID:    user,
			Title:     "new",
			CreatedAt: baseTime.Add(time.Minute),
			UpdatedAt: baseTime.Add(time.Hour),
		}))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 2, res.Total)
		require.Equal(t, newerID, res.Sessions[0].SessionID)
		require.Equal(t, olderID, res.Sessions[1].SessionID)
	})

	t.Run("List returns empty slice when no sessions exist", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: fake.Lorem().Word(),
			UserID:  fake.UUID().V4(),
			Limit:   5,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, res.Total)
		require.NotNil(t, res.Sessions)
		require.Empty(t, res.Sessions)
	})

	t.Run("List with offset skips entries total reflects full count", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := range 3 {
			require.NoError(t, store.Save(t.Context(), SessionMetadata{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				Title:     fake.Lorem().Word(),
				CreatedAt: ts.Add(time.Duration(i) * time.Minute),
				UpdatedAt: ts.Add(time.Duration(i+1) * time.Hour),
			}))
		}

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  1,
		})
		require.NoError(t, err)
		require.Equal(t, 3, res.Total)
		require.Len(t, res.Sessions, 2)
	})

	t.Run("List with limit caps results total reflects full count", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		ts := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		for i := range 5 {
			require.NoError(t, store.Save(t.Context(), SessionMetadata{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				Title:     fake.Lorem().Word(),
				CreatedAt: ts,
				UpdatedAt: ts.Add(time.Duration(i) * time.Minute),
			}))
		}

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   2,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 5, res.Total)
		require.Len(t, res.Sessions, 2)
	})

	t.Run("List with offset and limit together", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		ts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		ids := make([]string, 4)
		for i := range 4 {
			ids[i] = fake.UUID().V4()
			require.NoError(t, store.Save(t.Context(), SessionMetadata{
				SessionID: ids[i],
				AppName:   app,
				UserID:    user,
				Title:     fake.Lorem().Word(),
				CreatedAt: ts,
				UpdatedAt: ts.Add(time.Duration(i) * time.Hour),
			}))
		}

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   2,
			Offset:  1,
		})
		require.NoError(t, err)
		require.Equal(t, 4, res.Total)
		require.Len(t, res.Sessions, 2)
		// sorted desc by UpdatedAt: ids[3], ids[2], ids[1], ids[0]
		require.Equal(t, ids[2], res.Sessions[0].SessionID)
		require.Equal(t, ids[1], res.Sessions[1].SessionID)
	})

	t.Run("Delete removes entry", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     fake.Lorem().Word(),
			CreatedAt: now,
			UpdatedAt: now,
		}))

		require.NoError(t, store.Delete(t.Context(), app, user, sid))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, res.Total)
	})

	t.Run("Delete of non-existent session does not error", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)

		err = store.Delete(t.Context(), fake.Lorem().Word(), fake.UUID().V4(), fake.UUID().V4())
		require.NoError(t, err)
	})

	t.Run("index path layout", func(t *testing.T) {
		t.Parallel()
		base := filepath.Join(t.TempDir(), "root")
		app := "my-app"
		user := "user-1"
		require.Equal(t,
			filepath.Join(base, app, user, "_sessions_index.json"),
			sessionMetadataIndexPath(base, app, user),
		)
	})

	t.Run("NewFileSessionMetadataStore rejects empty base dir", func(t *testing.T) {
		t.Parallel()
		_, err := NewFileSessionMetadataStore("")
		require.Error(t, err)
	})

	t.Run("NewFileSessionMetadataStore fails when base path is not a directory", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		p := filepath.Join(t.TempDir(), fake.Lorem().Word())
		require.NoError(t, os.WriteFile(p, []byte("x"), 0600))
		_, err := NewFileSessionMetadataStore(p)
		require.Error(t, err)
	})

	t.Run("Save returns context error when cancelled", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err = store.Save(ctx, SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   fake.Lorem().Word(),
			UserID:    fake.UUID().V4(),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("List returns context error when cancelled", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = store.List(ctx, ListSessionMetadataParams{
			AppName: fake.Lorem().Word(),
			UserID:  fake.UUID().V4(),
			Limit:   1,
			Offset:  0,
		})
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Delete returns context error when cancelled", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err = store.Delete(ctx, fake.Lorem().Word(), fake.UUID().V4(), fake.UUID().V4())
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("Save validation errors", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		uid := fake.UUID().V4()
		app := fake.Lorem().Word()
		now := time.Now().UTC()

		err = store.Save(t.Context(), SessionMetadata{
			SessionID: "",
			AppName:   app,
			UserID:    uid,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.Error(t, err)

		err = store.Save(t.Context(), SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   "",
			UserID:    uid,
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.Error(t, err)

		err = store.Save(t.Context(), SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   app,
			UserID:    "",
			CreatedAt: now,
			UpdatedAt: now,
		})
		require.Error(t, err)
	})

	t.Run("List validation errors", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		uid := fake.UUID().V4()
		app := fake.Lorem().Word()

		_, err = store.List(t.Context(), ListSessionMetadataParams{AppName: "", UserID: uid, Limit: 1, Offset: 0})
		require.Error(t, err)
		_, err = store.List(t.Context(), ListSessionMetadataParams{AppName: app, UserID: "", Limit: 1, Offset: 0})
		require.Error(t, err)
		_, err = store.List(t.Context(), ListSessionMetadataParams{AppName: app, UserID: uid, Limit: 0, Offset: 0})
		require.Error(t, err)
		_, err = store.List(t.Context(), ListSessionMetadataParams{AppName: app, UserID: uid, Limit: 101, Offset: 0})
		require.Error(t, err)
		_, err = store.List(t.Context(), ListSessionMetadataParams{AppName: app, UserID: uid, Limit: 1, Offset: -1})
		require.Error(t, err)
	})

	t.Run("Delete validation errors", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		uid := fake.UUID().V4()
		app := fake.Lorem().Word()
		sid := fake.UUID().V4()

		err = store.Delete(t.Context(), "", uid, sid)
		require.Error(t, err)
		err = store.Delete(t.Context(), app, "", sid)
		require.Error(t, err)
		err = store.Delete(t.Context(), app, uid, "")
		require.Error(t, err)
	})

	t.Run("List fails when index path is not a file", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		idx := sessionMetadataIndexPath(base, app, user)
		require.NoError(t, os.MkdirAll(filepath.Dir(idx), 0750))
		require.NoError(t, os.Mkdir(idx, 0750))

		_, err = store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   1,
			Offset:  0,
		})
		require.Error(t, err)
	})

	t.Run("List fails on corrupt index JSON", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		idx := sessionMetadataIndexPath(base, app, user)
		require.NoError(t, os.MkdirAll(filepath.Dir(idx), 0750))
		require.NoError(t, os.WriteFile(idx, []byte("{not-json"), 0600))

		_, err = store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   1,
			Offset:  0,
		})
		require.Error(t, err)
	})

	t.Run("List offset beyond total returns empty page", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   app,
			UserID:    user,
			Title:     fake.Lorem().Word(),
			CreatedAt: now,
			UpdatedAt: now,
		}))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   5,
			Offset:  10,
		})
		require.NoError(t, err)
		require.Equal(t, 1, res.Total)
		require.Empty(t, res.Sessions)
	})

	t.Run("sort stable for equal UpdatedAt", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store, err := NewFileSessionMetadataStore(t.TempDir())
		require.NoError(t, err)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		ts := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
		idA := fake.UUID().V4()
		idB := fake.UUID().V4()
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: idA,
			AppName:   app,
			UserID:    user,
			Title:     "a",
			CreatedAt: ts,
			UpdatedAt: ts,
		}))
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: idB,
			AppName:   app,
			UserID:    user,
			Title:     "b",
			CreatedAt: ts,
			UpdatedAt: ts,
		}))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 2, res.Total)
	})

	t.Run("Save fails when index dir is not writable", func(t *testing.T) {
		t.Parallel()
		if os.Getuid() == 0 {
			t.Skip("root can write read-only dirs in some environments")
		}
		fake := faker.New()
		base := t.TempDir()
		store, err := NewFileSessionMetadataStore(base)
		require.NoError(t, err)
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   app,
			UserID:    user,
			Title:     fake.Lorem().Word(),
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, store.Save(t.Context(), meta))

		idx := sessionMetadataIndexPath(base, app, user)
		dir := filepath.Dir(idx)
		require.NoError(t, os.Chmod(dir, 0555))
		t.Cleanup(func() { _ = os.Chmod(dir, 0750) })

		err = store.Save(t.Context(), meta)
		require.Error(t, err)
	})
}
