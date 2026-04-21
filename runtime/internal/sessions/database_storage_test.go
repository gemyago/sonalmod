package sessions

import (
	"testing"
	"time"

	"github.com/gemyago/sonalmod/runtime/internal/gormsonal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
)

func TestNewDatabaseSessionsStorage(t *testing.T) {
	t.Parallel()
	fake := faker.New()

	t.Run(":memory: DSN returns storage without error", func(t *testing.T) {
		t.Parallel()
		s, err := NewDatabaseSessionsStorage(":memory:", gormsonal.GormSonalmodTablesOpts{})
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("empty DSN returns error", func(t *testing.T) {
		t.Parallel()
		s, err := NewDatabaseSessionsStorage("", gormsonal.GormSonalmodTablesOpts{})
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("AutoMigrate with :memory: succeeds", func(t *testing.T) {
		t.Parallel()
		s, err := NewDatabaseSessionsStorage(":memory:", gormsonal.GormSonalmodTablesOpts{})
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())
	})

	t.Run("after AutoMigrate Create succeeds", func(t *testing.T) {
		t.Parallel()
		s, err := NewDatabaseSessionsStorage(":memory:", gormsonal.GormSonalmodTablesOpts{})
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())

		ctx := t.Context()
		resp, err := s.Create(ctx, &session.CreateRequest{
			AppName:   fake.Lorem().Word(),
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
	})

	t.Run("meta is DatabaseSessionMetadataStore", func(t *testing.T) {
		t.Parallel()
		s, err := NewDatabaseSessionsStorage(":memory:", gormsonal.GormSonalmodTablesOpts{})
		require.NoError(t, err)
		require.NotNil(t, s.meta)
		_, ok := any(s.meta).(*DatabaseSessionMetadataStore)
		require.True(t, ok)
	})

	t.Run("SaveMetadata ListMetadata DeleteMetadata delegate to database store", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		s, err := NewDatabaseSessionsStorage(":memory:", gormsonal.GormSonalmodTablesOpts{})
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())

		app := "app-db"
		user := "user-db"
		sid := "sess-db"
		now := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "title",
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, s.SaveMetadata(ctx, meta))

		listed, err := s.ListMetadata(ctx, ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 1, listed.Total)
		require.Equal(t, meta, listed.Sessions[0])

		require.NoError(t, s.DeleteMetadata(ctx, app, user, sid))
		after, err := s.ListMetadata(ctx, ListSessionMetadataParams{
			AppName: app,
			UserID:  user,
			Limit:   10,
			Offset:  0,
		})
		require.NoError(t, err)
		require.Equal(t, 0, after.Total)
	})
}
