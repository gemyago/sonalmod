package sessions

import (
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
)

func TestNewMemorySessionsStorage(t *testing.T) {
	t.Parallel()

	t.Run("returns non-nil", func(t *testing.T) {
		t.Parallel()
		s := NewMemorySessionsStorage()
		require.NotNil(t, s)
	})

	t.Run("AutoMigrate returns nil", func(t *testing.T) {
		t.Parallel()
		s := NewMemorySessionsStorage()
		require.NoError(t, s.AutoMigrate())
	})
}

func TestMemorySessionMetadataStore(t *testing.T) {
	t.Parallel()

	t.Run("Save and List round-trip", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     fake.Lorem().Sentence(3),
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, store.Save(t.Context(), meta))

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

	t.Run("Save upserts same session id", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		t0 := time.Now().UTC().Truncate(time.Second)
		first := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "a",
			CreatedAt: t0,
			UpdatedAt: t0,
		}
		require.NoError(t, store.Save(t.Context(), first))
		second := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "b",
			CreatedAt: t0,
			UpdatedAt: t0.Add(time.Minute),
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
		require.Equal(t, second.Title, res.Sessions[0].Title)
	})

	t.Run("List sorts by UpdatedAt descending", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		tOld := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
		tNew := time.Now().UTC().Truncate(time.Second)
		oldMeta := SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   app,
			UserID:    user,
			Title:     "old",
			CreatedAt: tOld,
			UpdatedAt: tOld,
		}
		newMeta := SessionMetadata{
			SessionID: fake.UUID().V4(),
			AppName:   app,
			UserID:    user,
			Title:     "new",
			CreatedAt: tNew,
			UpdatedAt: tNew,
		}
		require.NoError(t, store.Save(t.Context(), oldMeta))
		require.NoError(t, store.Save(t.Context(), newMeta))

		res, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 2, res.Total)
		require.Equal(t, newMeta.SessionID, res.Sessions[0].SessionID)
		require.Equal(t, oldMeta.SessionID, res.Sessions[1].SessionID)
	})

	t.Run("List applies offset and limit", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		base := time.Now().UTC().Truncate(time.Second)
		sessions := []SessionMetadata{
			{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				Title:     "t0",
				CreatedAt: base,
				UpdatedAt: base,
			},
			{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				Title:     "t1",
				CreatedAt: base,
				UpdatedAt: base.Add(time.Hour),
			},
			{
				SessionID: fake.UUID().V4(),
				AppName:   app,
				UserID:    user,
				Title:     "t2",
				CreatedAt: base,
				UpdatedAt: base.Add(2 * time.Hour),
			},
		}
		for _, m := range sessions {
			require.NoError(t, store.Save(t.Context(), m))
		}
		all, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 3, all.Total)

		page, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   1,
			Offset:  1,
		})
		require.NoError(t, err)
		require.Equal(t, 3, page.Total)
		require.Len(t, page.Sessions, 1)
		require.Equal(t, "t1", page.Sessions[0].Title)
	})

	t.Run("Delete removes entry", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		app := fake.Lorem().Word()
		user := fake.UUID().V4()
		sid := fake.UUID().V4()
		now := time.Now().UTC().Truncate(time.Second)
		require.NoError(t, store.Save(t.Context(), SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "x",
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

	t.Run("Delete of missing session does not error", func(t *testing.T) {
		t.Parallel()
		fake := faker.New()
		store := NewMemorySessionMetadataStore()
		require.NoError(t, store.Delete(t.Context(), fake.Lorem().Word(), fake.UUID().V4(), fake.UUID().V4()))
	})

	t.Run("Save rejects invalid metadata", func(t *testing.T) {
		t.Parallel()
		store := NewMemorySessionMetadataStore()
		err := store.Save(t.Context(), SessionMetadata{})
		require.Error(t, err)
	})

	t.Run("List rejects invalid params", func(t *testing.T) {
		t.Parallel()
		store := NewMemorySessionMetadataStore()
		_, err := store.List(t.Context(), ListSessionMetadataParams{
			AppName: "",
			UserID:  "",
			Limit:   10,
			Offset:  0,
		})
		require.Error(t, err)
	})

	t.Run("Delete rejects empty ids", func(t *testing.T) {
		t.Parallel()
		store := NewMemorySessionMetadataStore()
		require.Error(t, store.Delete(t.Context(), "", "u", "s"))
		require.Error(t, store.Delete(t.Context(), "a", "u", ""))
	})
}
