package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/system/ident"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/dig"
)

func TestUserCmd(t *testing.T) {
	fake := faker.New()

	type userCmdDeps struct {
		store     *auth.UserStore
		hasher    *auth.Argon2idHasher
		container *dig.Container
	}

	makeUserCmdDeps := func(t *testing.T) userCmdDeps {
		t.Helper()
		return userCmdDeps{
			store: auth.NewUserStore(auth.UserStoreDeps{
				DataDir: t.TempDir(),
				IDGen:   ident.NewDefaultGenerator(),
				Logger:  slog.Default(),
			}),
			hasher:    auth.NewArgon2idHasher(),
			container: dig.New(),
		}
	}

	makeBrokenStore := func(t *testing.T) *auth.UserStore {
		t.Helper()
		// Use a data dir where the users subdirectory is a file (not a dir), causing List to fail.
		dataDir := t.TempDir()
		usersPath := filepath.Join(dataDir, "auth", "users")
		require.NoError(t, os.MkdirAll(filepath.Join(dataDir, "auth"), 0o700))
		require.NoError(t, os.WriteFile(usersPath, []byte("not a dir"), 0o600))
		return auth.NewUserStore(auth.UserStoreDeps{
			DataDir: dataDir,
			IDGen:   ident.NewDefaultGenerator(),
			Logger:  slog.Default(),
		})
	}

	t.Run("newUserCmd", func(t *testing.T) {
		t.Run("has correct Use", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			cmd := newUserCmd(deps.container)
			assert.Equal(t, "user", cmd.Use)
		})

		t.Run("has add, list, change-password subcommands", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			cmd := newUserCmd(deps.container)
			names := make([]string, 0)
			for _, sub := range cmd.Commands() {
				names = append(names, sub.Name())
			}
			assert.Contains(t, names, "add")
			assert.Contains(t, names, "list")
			assert.Contains(t, names, "change-password")
		})
	})

	t.Run("runUserAdd", func(t *testing.T) {
		t.Run("creates user and prints ID", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher
			username := fake.Internet().User()
			password := fake.Internet().Password()

			var buf bytes.Buffer
			err := runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: password,
			}, &buf)
			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, username)

			// Verify user is actually stored
			user, getErr := store.GetByUsername(t.Context(), username)
			require.NoError(t, getErr)
			assert.Equal(t, username, user.Username)
		})

		t.Run("returns error for duplicate username", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher
			username := fake.Internet().User()
			password := fake.Internet().Password()

			var buf bytes.Buffer
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: password,
			}, &buf))

			var buf2 bytes.Buffer
			err := runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: fake.Internet().Password(),
			}, &buf2)
			require.ErrorIs(t, err, auth.ErrUsernameExists)
		})
	})

	t.Run("runUserList", func(t *testing.T) {
		t.Run("lists all created users", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher

			username1 := fake.Internet().User() + "1"
			username2 := fake.Internet().User() + "2"

			var buf bytes.Buffer
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username1,
				Password: fake.Internet().Password(),
			}, &buf))
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username2,
				Password: fake.Internet().Password(),
			}, &buf))

			var listBuf bytes.Buffer
			err := runUserList(t.Context(), userListCmdDeps{Store: store}, &listBuf)
			require.NoError(t, err)

			output := listBuf.String()
			assert.Contains(t, output, username1)
			assert.Contains(t, output, username2)
			assert.NotContains(t, output, "passwordHash", "password hash should not be printed")
		})

		t.Run("prints header even when no users", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store

			var listBuf bytes.Buffer
			err := runUserList(t.Context(), userListCmdDeps{Store: store}, &listBuf)
			require.NoError(t, err)

			output := listBuf.String()
			// Should at minimum print something (header or empty message)
			assert.NotEmpty(t, output)
		})

		t.Run("does not print password hashes or algorithm name", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher
			username := fake.Internet().User()
			password := fake.Internet().Password()

			var buf bytes.Buffer
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: password,
			}, &buf))

			var listBuf bytes.Buffer
			require.NoError(t, runUserList(t.Context(), userListCmdDeps{Store: store}, &listBuf))

			output := listBuf.String()
			user, err := store.GetByUsername(t.Context(), username)
			require.NoError(t, err)
			assert.NotContains(t, output, user.PasswordHash)
			assert.NotContains(t, output, "argon2id", "hash should not be in list output")
		})
	})

	t.Run("runUserChangePassword", func(t *testing.T) {
		t.Run("updates password for existing user", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher
			username := fake.Internet().User()
			oldPassword := fake.Internet().Password()
			newPassword := fake.Internet().Password()

			var buf bytes.Buffer
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: oldPassword,
			}, &buf))

			var changeBuf bytes.Buffer
			cpDeps := userChangePasswordCmdDeps{Store: store, Hasher: hasher}
			err := runUserChangePassword(t.Context(), cpDeps, userChangePasswordParams{
				Username: username,
				Password: newPassword,
			}, &changeBuf)
			require.NoError(t, err)

			// Verify password changed
			user, getErr := store.GetByUsername(t.Context(), username)
			require.NoError(t, getErr)

			ok, verifyErr := hasher.Verify(newPassword, user.PasswordHash)
			require.NoError(t, verifyErr)
			assert.True(t, ok, "new password should verify")

			notOk, verifyOldErr := hasher.Verify(oldPassword, user.PasswordHash)
			require.NoError(t, verifyOldErr)
			assert.False(t, notOk, "old password should not verify")
		})

		t.Run("returns error for non-existent user", func(t *testing.T) {
			deps := makeUserCmdDeps(t)
			store := deps.store
			hasher := deps.hasher

			var buf bytes.Buffer
			cpDeps := userChangePasswordCmdDeps{Store: store, Hasher: hasher}
			err := runUserChangePassword(t.Context(), cpDeps, userChangePasswordParams{
				Username: fake.Internet().User(),
				Password: fake.Internet().Password(),
			}, &buf)
			require.ErrorIs(t, err, auth.ErrUserNotFound)
		})
	})

	t.Run("end-to-end via cobra command", func(t *testing.T) {
		t.Run("user add and user list work together", func(t *testing.T) {
			chdirModuleRoot(t)
			// -e test uses dataDir on disk under apps/sonalmod; faker usernames can collide with
			// users left from earlier runs or other tests, so the name must be unique per run.
			username := fmt.Sprintf("%s_%d", fake.Internet().User(), time.Now().UnixNano())
			password := fake.Internet().Password()
			logf := testLogFile(t)

			// Each Execute builds an engine on the shared dig.Container; use a fresh command tree
			// per run so provider registration is not duplicated.
			rootAdd := setupCommands()
			rootAdd.SilenceUsage = true
			rootAdd.SilenceErrors = true
			var addBuf bytes.Buffer
			rootAdd.SetOut(&addBuf)
			rootAdd.SetArgs([]string{
				"user", "add", "-e", "test", "--username", username, "--password", password, "--logs-file", logf,
			})
			require.NoError(t, rootAdd.ExecuteContext(t.Context()))

			rootList := setupCommands()
			rootList.SilenceUsage = true
			rootList.SilenceErrors = true
			var listBuf bytes.Buffer
			rootList.SetOut(&listBuf)
			rootList.SetArgs([]string{"user", "list", "-e", "test", "--logs-file", logf})
			require.NoError(t, rootList.ExecuteContext(t.Context()))

			assert.Contains(t, listBuf.String(), username)
		})
	})

	t.Run("error paths", func(t *testing.T) {
		t.Run("runUserList propagates store.List error", func(t *testing.T) {
			store := makeBrokenStore(t)
			var buf bytes.Buffer
			err := runUserList(t.Context(), userListCmdDeps{Store: store}, &buf)
			require.Error(t, err)
			assert.ErrorContains(t, err, "list users")
		})

		t.Run("runUserChangePassword propagates UpdatePassword error", func(t *testing.T) {
			dataDir := t.TempDir()
			store := auth.NewUserStore(auth.UserStoreDeps{
				DataDir: dataDir,
				IDGen:   ident.NewDefaultGenerator(),
				Logger:  slog.Default(),
			})
			hasher := auth.NewArgon2idHasher()
			username := fake.Internet().User()

			var buf bytes.Buffer
			require.NoError(t, runUserAdd(t.Context(), userAddCmdDeps{Store: store, Hasher: hasher}, userAddParams{
				Username: username,
				Password: fake.Internet().Password(),
			}, &buf))

			// Make the users directory read-only so UpdatePassword cannot write the updated file.
			usersDir := filepath.Join(dataDir, "auth", "users")
			require.NoError(t, os.Chmod(usersDir, 0o500))
			t.Cleanup(func() { _ = os.Chmod(usersDir, 0o700) })

			var changeBuf bytes.Buffer
			cpDeps := userChangePasswordCmdDeps{Store: store, Hasher: hasher}
			err := runUserChangePassword(t.Context(), cpDeps, userChangePasswordParams{
				Username: username,
				Password: fake.Internet().Password(),
			}, &changeBuf)
			require.Error(t, err)
		})
	})
}
