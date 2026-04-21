package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/system/ident"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
)

func TestUserStore(t *testing.T) {
	fake := faker.New()

	makeStore := func(t *testing.T) *UserStore {
		t.Helper()
		return NewUserStore(UserStoreDeps{
			DataDir: t.TempDir(),
			IDGen:   ident.NewDefaultGenerator(),
			Logger:  telemetry.RootTestLogger(),
		})
	}

	makeCreateParams := func() CreateUserParams {
		return CreateUserParams{
			Username:     fake.Internet().User(),
			PasswordHash: fake.Lorem().Text(60),
		}
	}

	t.Run("Create", func(t *testing.T) {
		t.Run("creates user and returns it", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			user, err := store.Create(t.Context(), params)

			require.NoError(t, err)
			assert.NotEmpty(t, user.ID)
			assert.Equal(t, params.Username, user.Username)
			assert.Equal(t, params.PasswordHash, user.PasswordHash)
			assert.NotZero(t, user.CreatedAt)
			assert.NotZero(t, user.UpdatedAt)
		})

		t.Run("returns ErrUsernameExists for duplicate username", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			_, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			_, err = store.Create(t.Context(), params)
			require.ErrorIs(t, err, ErrUsernameExists)
		})

		t.Run("allows different usernames", func(t *testing.T) {
			store := makeStore(t)

			params1 := makeCreateParams()
			params2 := makeCreateParams()

			_, err := store.Create(t.Context(), params1)
			require.NoError(t, err)

			_, err = store.Create(t.Context(), params2)
			require.NoError(t, err)
		})
	})

	t.Run("GetByID", func(t *testing.T) {
		t.Run("returns created user by ID", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			created, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			got, err := store.GetByID(t.Context(), created.ID)
			require.NoError(t, err)
			assert.Equal(t, created, got)
		})

		t.Run("returns ErrUserNotFound for non-existent ID", func(t *testing.T) {
			store := makeStore(t)

			_, err := store.GetByID(t.Context(), fake.UUID().V4())
			require.ErrorIs(t, err, ErrUserNotFound)
		})
	})

	t.Run("GetByUsername", func(t *testing.T) {
		t.Run("returns created user by username", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			created, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			got, err := store.GetByUsername(t.Context(), params.Username)
			require.NoError(t, err)
			assert.Equal(t, created, got)
		})

		t.Run("returns ErrUserNotFound for non-existent username", func(t *testing.T) {
			store := makeStore(t)

			_, err := store.GetByUsername(t.Context(), fake.Internet().User())
			require.ErrorIs(t, err, ErrUserNotFound)
		})
	})

	t.Run("List", func(t *testing.T) {
		t.Run("returns empty slice when no users", func(t *testing.T) {
			store := makeStore(t)

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Empty(t, users)
		})

		t.Run("returns all created users", func(t *testing.T) {
			store := makeStore(t)

			created1, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			created2, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Len(t, users, 2)
			assert.ElementsMatch(t, []User{*created1, *created2}, users)
		})
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		t.Run("updates the password hash", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			created, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			newHash := fake.Lorem().Text(60)
			err = store.UpdatePassword(t.Context(), created.ID, newHash)
			require.NoError(t, err)

			got, err := store.GetByID(t.Context(), created.ID)
			require.NoError(t, err)
			assert.Equal(t, newHash, got.PasswordHash)
			assert.Equal(t, created.Username, got.Username)
			assert.Equal(t, created.CreatedAt, got.CreatedAt)
			assert.True(t, got.UpdatedAt.After(created.UpdatedAt) || got.UpdatedAt.Equal(created.UpdatedAt))
		})

		t.Run("returns ErrUserNotFound for non-existent user", func(t *testing.T) {
			store := makeStore(t)

			err := store.UpdatePassword(t.Context(), fake.UUID().V4(), fake.Lorem().Text(60))
			require.ErrorIs(t, err, ErrUserNotFound)
		})

		t.Run("returns error when write fails", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			created, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			// Make directory read-only so write fails.
			usersDir := filepath.Join(store.deps.DataDir, "auth", "users")
			require.NoError(t, os.Chmod(usersDir, 0o500))
			t.Cleanup(func() { _ = os.Chmod(usersDir, 0o700) })

			err = store.UpdatePassword(t.Context(), created.ID, fake.Lorem().Text(60))
			require.Error(t, err)
		})
	})

	t.Run("List error paths", func(t *testing.T) {
		t.Run("returns error when directory is not readable", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			// Create a user to ensure the directory exists.
			_, err := store.Create(t.Context(), params)
			require.NoError(t, err)

			// Make auth dir non-readable so ReadDir fails (not ErrNotExist).
			authDir := filepath.Join(store.deps.DataDir, "auth")
			require.NoError(t, os.Chmod(authDir, 0o000))
			t.Cleanup(func() { _ = os.Chmod(authDir, 0o700) })

			_, err = store.List(t.Context())
			require.Error(t, err)
		})

		t.Run("skips directories inside users dir", func(t *testing.T) {
			store := makeStore(t)

			// Create a user to ensure the directory exists.
			created, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			// Create a subdirectory inside users dir.
			subDir := filepath.Join(store.deps.DataDir, "auth", "users", "subdir")
			require.NoError(t, os.MkdirAll(subDir, 0o700))

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []User{*created}, users)
		})

		t.Run("skips non-JSON files", func(t *testing.T) {
			store := makeStore(t)

			created, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			// Write a non-JSON file.
			nonJSONFile := filepath.Join(store.deps.DataDir, "auth", "users", "somefile.txt")
			require.NoError(t, os.WriteFile(nonJSONFile, []byte("data"), 0o600))

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []User{*created}, users)
		})

		t.Run("skips temp files", func(t *testing.T) {
			store := makeStore(t)

			created, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			// Write a temp JSON file that would match the pattern.
			tmpFile := filepath.Join(store.deps.DataDir, "auth", "users", fake.UUID().V4()+".tmp.json")
			require.NoError(t, os.WriteFile(tmpFile, []byte("{}"), 0o600))

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []User{*created}, users)
		})

		t.Run("skips corrupt JSON files and logs warning", func(t *testing.T) {
			store := makeStore(t)

			created, err := store.Create(t.Context(), makeCreateParams())
			require.NoError(t, err)

			// Write a corrupt JSON file.
			corruptFile := filepath.Join(store.deps.DataDir, "auth", "users", fake.UUID().V4()+".json")
			require.NoError(t, os.WriteFile(corruptFile, []byte("not valid json"), 0o600))

			users, err := store.List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, []User{*created}, users)
		})
	})

	t.Run("GetByID error paths", func(t *testing.T) {
		t.Run("returns error for corrupt JSON file", func(t *testing.T) {
			store := makeStore(t)

			corruptID := fake.UUID().V4()
			corruptFile := filepath.Join(store.deps.DataDir, "auth", "users", corruptID+".json")
			usersDir := filepath.Join(store.deps.DataDir, "auth", "users")
			require.NoError(t, os.MkdirAll(usersDir, 0o700))
			require.NoError(t, os.WriteFile(corruptFile, []byte("not valid json"), 0o600))

			_, err := store.GetByID(t.Context(), corruptID)
			require.Error(t, err)
		})
	})

	t.Run("Create error paths", func(t *testing.T) {
		t.Run("returns error when write fails", func(t *testing.T) {
			store := makeStore(t)
			params := makeCreateParams()

			// Create the directory first so it exists but is then made read-only.
			usersDir := filepath.Join(store.deps.DataDir, "auth", "users")
			require.NoError(t, os.MkdirAll(usersDir, 0o700))
			require.NoError(t, os.Chmod(usersDir, 0o500))
			t.Cleanup(func() { _ = os.Chmod(usersDir, 0o700) })

			_, err := store.Create(t.Context(), params)
			require.Error(t, err)
		})
	})
}
