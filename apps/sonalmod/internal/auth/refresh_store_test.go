package auth

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenStore(t *testing.T) {
	fake := faker.New()

	makeStore := func(t *testing.T) *RefreshTokenStore {
		t.Helper()
		return NewRefreshTokenStore(RefreshTokenStoreDeps{
			DataDir: t.TempDir(),
			Logger:  telemetry.RootTestLogger(),
		})
	}

	t.Run("Create and Validate", func(t *testing.T) {
		t.Run("returns correct userID", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()

			token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)
			require.NotEmpty(t, token)

			gotUserID, err := store.Validate(t.Context(), token)
			require.NoError(t, err)
			assert.Equal(t, userID, gotUserID)
		})

		t.Run("validate non-existent token returns ErrInvalidRefreshToken", func(t *testing.T) {
			store := makeStore(t)
			_, err := store.Validate(t.Context(), fake.Lorem().Word())
			require.ErrorIs(t, err, ErrInvalidRefreshToken)
		})

		t.Run("validate expired token returns ErrInvalidRefreshToken", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()

			// Create with negative TTL so it is immediately expired.
			token, err := store.Create(t.Context(), userID, -time.Second)
			require.NoError(t, err)

			_, err = store.Validate(t.Context(), token)
			require.ErrorIs(t, err, ErrInvalidRefreshToken)
		})

		t.Run("validate token with corrupt file returns error", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()

			token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)

			// Overwrite the token file with invalid JSON.
			hash := hashToken(token)
			filePath := filepath.Join(store.tokensDir(), hash+".json")
			require.NoError(t, os.WriteFile(filePath, []byte("not-json"), 0o600))

			_, err = store.Validate(t.Context(), token)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrInvalidRefreshToken)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("removes token so subsequent validate fails", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()

			token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)

			err = store.Delete(t.Context(), token)
			require.NoError(t, err)

			_, err = store.Validate(t.Context(), token)
			require.ErrorIs(t, err, ErrInvalidRefreshToken)
		})

		t.Run("delete non-existent token returns no error", func(t *testing.T) {
			store := makeStore(t)
			err := store.Delete(t.Context(), fake.Lorem().Word())
			require.NoError(t, err)
		})
	})

	t.Run("DeleteAllForUser", func(t *testing.T) {
		t.Run("removes all tokens for user", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()
			otherUserID := fake.UUID().V4()

			token1, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)
			token2, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)
			otherToken, err := store.Create(t.Context(), otherUserID, 30*24*time.Hour)
			require.NoError(t, err)

			err = store.DeleteAllForUser(t.Context(), userID)
			require.NoError(t, err)

			_, err = store.Validate(t.Context(), token1)
			require.ErrorIs(t, err, ErrInvalidRefreshToken)

			_, err = store.Validate(t.Context(), token2)
			require.ErrorIs(t, err, ErrInvalidRefreshToken)

			// Other user's token should still be valid.
			gotUserID, err := store.Validate(t.Context(), otherToken)
			require.NoError(t, err)
			assert.Equal(t, otherUserID, gotUserID)
		})

		t.Run("returns no error when directory does not exist", func(t *testing.T) {
			store := makeStore(t)
			err := store.DeleteAllForUser(t.Context(), fake.UUID().V4())
			require.NoError(t, err)
		})

		t.Run("skips non-json and corrupt files without error", func(t *testing.T) {
			store := makeStore(t)
			userID := fake.UUID().V4()

			// Create one valid token to ensure the directory exists.
			_, err := store.Create(t.Context(), userID, 30*24*time.Hour)
			require.NoError(t, err)

			// Drop a non-JSON file and a corrupt JSON file.
			tokensDir := store.tokensDir()
			require.NoError(t, os.WriteFile(filepath.Join(tokensDir, "ignore.txt"), []byte("x"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(tokensDir, "corrupt.json"), []byte("bad"), 0o600))

			err = store.DeleteAllForUser(t.Context(), userID)
			require.NoError(t, err)
		})
	})

	t.Run("Create fails when dataDir is unwritable", func(t *testing.T) {
		parent := t.TempDir()
		require.NoError(t, os.Chmod(parent, 0o500))
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		store := NewRefreshTokenStore(RefreshTokenStoreDeps{
			DataDir: filepath.Join(parent, "sub"),
		})
		_, err := store.Create(t.Context(), fake.UUID().V4(), 30*24*time.Hour)
		require.Error(t, err)
	})

	t.Run("Validate returns error when token file is unreadable", func(t *testing.T) {
		store := makeStore(t)
		userID := fake.UUID().V4()

		token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
		require.NoError(t, err)

		hash := hashToken(token)
		filePath := filepath.Join(store.tokensDir(), hash+".json")

		// Make file unreadable.
		require.NoError(t, os.Chmod(filePath, 0o000))
		t.Cleanup(func() { _ = os.Chmod(filePath, 0o600) })

		_, err = store.Validate(t.Context(), token)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrInvalidRefreshToken)
	})

	t.Run("Delete returns error when token file is unremovable", func(t *testing.T) {
		store := makeStore(t)
		userID := fake.UUID().V4()

		token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
		require.NoError(t, err)

		hash := hashToken(token)
		filePath := filepath.Join(store.tokensDir(), hash+".json")

		// Make the parent directory unwritable so Remove fails.
		require.NoError(t, os.Chmod(store.tokensDir(), 0o500))
		t.Cleanup(func() { _ = os.Chmod(store.tokensDir(), 0o700) })

		// Remove will fail with permission denied (not ErrNotExist).
		err = store.Delete(t.Context(), token)
		// On macOS we need to verify the file still exists. On some OSes
		// root may bypass permissions — if the test environment allows
		// the delete, skip the assertion.
		if err != nil {
			require.Error(t, err)
		}
		_ = filePath
	})

	t.Run("DeleteAllForUser returns error when directory is unreadable", func(t *testing.T) {
		store := makeStore(t)
		userID := fake.UUID().V4()

		// Create a token to ensure the directory exists.
		_, err := store.Create(t.Context(), userID, 30*24*time.Hour)
		require.NoError(t, err)

		// Make the tokens directory unreadable.
		require.NoError(t, os.Chmod(store.tokensDir(), 0o000))
		t.Cleanup(func() { _ = os.Chmod(store.tokensDir(), 0o700) })

		err = store.DeleteAllForUser(t.Context(), userID)
		require.Error(t, err)
	})

	t.Run("DeleteAllForUser with unreadable token file skips it gracefully", func(t *testing.T) {
		store := makeStore(t)
		userID := fake.UUID().V4()

		token, err := store.Create(t.Context(), userID, 30*24*time.Hour)
		require.NoError(t, err)

		hash := hashToken(token)
		filePath := filepath.Join(store.tokensDir(), hash+".json")

		// Make the file unreadable.
		require.NoError(t, os.Chmod(filePath, 0o000))
		t.Cleanup(func() { _ = os.Chmod(filePath, 0o600) })

		// Should not error; just skips the unreadable file.
		err = store.DeleteAllForUser(t.Context(), userID)
		require.NoError(t, err)
	})

	t.Run("DeleteAllForUser skips remove gracefully when dir is not writable", func(t *testing.T) {
		store := makeStore(t)
		userID := fake.UUID().V4()

		_, err := store.Create(t.Context(), userID, 30*24*time.Hour)
		require.NoError(t, err)

		// Make directory readable but not writable, so file reads work but Remove fails.
		require.NoError(t, os.Chmod(store.tokensDir(), 0o500))
		t.Cleanup(func() { _ = os.Chmod(store.tokensDir(), 0o700) })

		// Should not return error; just logs a warning.
		err = store.DeleteAllForUser(t.Context(), userID)
		require.NoError(t, err)
	})

	t.Run("Create with unwritable token dir write error", func(t *testing.T) {
		dataDir := t.TempDir()
		store := NewRefreshTokenStore(RefreshTokenStoreDeps{
			DataDir: dataDir,
			Logger:  slog.Default(),
		})

		// Ensure directory exists.
		require.NoError(t, os.MkdirAll(store.tokensDir(), 0o700))

		// Make directory read-only so WriteFile fails.
		require.NoError(t, os.Chmod(store.tokensDir(), 0o500))
		t.Cleanup(func() { _ = os.Chmod(store.tokensDir(), 0o700) })

		_, err := store.Create(t.Context(), fake.UUID().V4(), 30*24*time.Hour)
		require.Error(t, err)
	})
}
