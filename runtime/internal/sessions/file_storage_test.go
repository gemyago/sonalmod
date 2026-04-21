package sessions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewFileSessionsStorage(t *testing.T) {
	t.Parallel()

	t.Run("valid dir returns non-nil and no error", func(t *testing.T) {
		t.Parallel()
		baseDir := t.TempDir()
		s, err := NewFileSessionsStorage(baseDir, nil)
		require.NoError(t, err)
		require.NotNil(t, s)
	})

	t.Run("empty dir returns error", func(t *testing.T) {
		t.Parallel()
		s, err := NewFileSessionsStorage("", nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("AutoMigrate returns nil", func(t *testing.T) {
		t.Parallel()
		s, err := NewFileSessionsStorage(t.TempDir(), nil)
		require.NoError(t, err)
		require.NoError(t, s.AutoMigrate())
	})

	t.Run("embeds concrete FileSessionService", func(t *testing.T) {
		t.Parallel()
		s, err := NewFileSessionsStorage(t.TempDir(), nil)
		require.NoError(t, err)
		require.NotNil(t, s.FileSessionService)
		_, ok := any(s.FileSessionService).(*FileSessionService)
		require.True(t, ok, "embedded field should be *FileSessionService")
	})

	t.Run("meta is FileSessionMetadataStore", func(t *testing.T) {
		t.Parallel()
		s, err := NewFileSessionsStorage(t.TempDir(), nil)
		require.NoError(t, err)
		require.NotNil(t, s.meta)
		_, ok := any(s.meta).(*FileSessionMetadataStore)
		require.True(t, ok, "meta should be *FileSessionMetadataStore")
	})

	t.Run("SaveMetadata ListMetadata DeleteMetadata delegate to file store", func(t *testing.T) {
		t.Parallel()
		ctx := t.Context()
		baseDir := t.TempDir()
		s, err := NewFileSessionsStorage(baseDir, nil)
		require.NoError(t, err)

		app := "app-a"
		user := "user-1"
		sid := "sess-1"
		now := time.Now().UTC().Truncate(time.Second)
		meta := SessionMetadata{
			SessionID: sid,
			AppName:   app,
			UserID:    user,
			Title:     "hello",
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
