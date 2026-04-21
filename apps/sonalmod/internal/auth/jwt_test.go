package auth

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTService(t *testing.T) {
	fake := faker.New()

	makeService := func(t *testing.T, signingKey string) *JWTService {
		t.Helper()
		svc, err := NewJWTService(JWTServiceDeps{
			SigningKey:     signingKey,
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        t.TempDir(),
		})
		require.NoError(t, err)
		return svc
	}

	t.Run("GenerateAccessToken and ValidateAccessToken", func(t *testing.T) {
		t.Run("returns correct claims", func(t *testing.T) {
			svc := makeService(t, fake.Lorem().Word())
			userID := fake.UUID().V4()
			username := fake.Internet().User()

			token, err := svc.GenerateAccessToken(userID, username)
			require.NoError(t, err)
			require.NotEmpty(t, token)

			claims, err := svc.ValidateAccessToken(token)
			require.NoError(t, err)
			require.NotNil(t, claims)
			assert.Equal(t, userID, claims.Subject)
			assert.Equal(t, username, claims.Username)
		})

		t.Run("validate expired token returns error", func(t *testing.T) {
			svc, err := NewJWTService(JWTServiceDeps{
				SigningKey:     fake.Lorem().Word(),
				AccessTokenTTL: -time.Second, // already expired
				DataDir:        t.TempDir(),
			})
			require.NoError(t, err)

			token, err := svc.GenerateAccessToken(fake.UUID().V4(), fake.Internet().User())
			require.NoError(t, err)

			_, err = svc.ValidateAccessToken(token)
			require.Error(t, err)
		})

		t.Run("validate tampered token returns error", func(t *testing.T) {
			svc := makeService(t, fake.Lorem().Word())
			token, err := svc.GenerateAccessToken(fake.UUID().V4(), fake.Internet().User())
			require.NoError(t, err)

			tampered := token + "x"
			_, err = svc.ValidateAccessToken(tampered)
			require.Error(t, err)
		})

		t.Run("validate token with wrong key returns error", func(t *testing.T) {
			svc1 := makeService(t, fake.Lorem().Word()+"key1")
			svc2 := makeService(t, fake.Lorem().Word()+"key2")

			token, err := svc1.GenerateAccessToken(fake.UUID().V4(), fake.Internet().User())
			require.NoError(t, err)

			_, err = svc2.ValidateAccessToken(token)
			require.Error(t, err)
		})

		t.Run("validate completely invalid token returns error", func(t *testing.T) {
			svc := makeService(t, fake.Lorem().Word())
			_, err := svc.ValidateAccessToken(fake.Lorem().Sentence(3))
			require.Error(t, err)
		})

		t.Run("validate token with non-HMAC signing method returns error", func(t *testing.T) {
			svc := makeService(t, fake.Lorem().Word())

			// Craft a JWT header+payload using RS256 alg.
			// The keyfunc rejects it because it's not *jwt.SigningMethodHMAC.
			header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
			payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"123"}`))
			// Signature is garbage; the keyfunc fires before signature is checked.
			sig := base64.RawURLEncoding.EncodeToString([]byte("fake-sig"))
			token := header + "." + payload + "." + sig

			_, err := svc.ValidateAccessToken(token)
			require.Error(t, err)
		})
	})

	t.Run("NewJWTService with unwritable dataDir returns error", func(t *testing.T) {
		// Make a read-only parent so MkdirAll fails for the sub-directory.
		parent := t.TempDir()
		require.NoError(t, os.Chmod(parent, 0o500))
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		unwritableDataDir := filepath.Join(parent, "sub")
		_, err := NewJWTService(JWTServiceDeps{
			SigningKey:     "",
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        unwritableDataDir,
		})
		require.Error(t, err)
	})

	t.Run("auto-generated key persists and is reused", func(t *testing.T) {
		dataDir := t.TempDir()

		svc1, err := NewJWTService(JWTServiceDeps{
			SigningKey:     "",
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        dataDir,
		})
		require.NoError(t, err)

		token, err := svc1.GenerateAccessToken(fake.UUID().V4(), fake.Internet().User())
		require.NoError(t, err)

		keyFilePath := filepath.Join(dataDir, "auth", "jwt-signing-key")
		_, statErr := os.Stat(keyFilePath)
		require.NoError(t, statErr, "jwt-signing-key file should exist")

		svc2, err := NewJWTService(JWTServiceDeps{
			SigningKey:     "",
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        dataDir,
		})
		require.NoError(t, err)

		claims, err := svc2.ValidateAccessToken(token)
		require.NoError(t, err)
		require.NotNil(t, claims)
	})

	t.Run("NewJWTService errors when key file is unreadable", func(t *testing.T) {
		dataDir := t.TempDir()
		authDir := filepath.Join(dataDir, "auth")
		require.NoError(t, os.MkdirAll(authDir, 0o700))

		// Write the key file then make it unreadable.
		keyFile := filepath.Join(authDir, "jwt-signing-key")
		require.NoError(t, os.WriteFile(keyFile, []byte("somekey"), 0o000))
		t.Cleanup(func() { _ = os.Chmod(keyFile, 0o600) })

		_, err := NewJWTService(JWTServiceDeps{
			SigningKey:     "",
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        dataDir,
		})
		require.Error(t, err)
	})

	t.Run("NewJWTService errors when key file cannot be written", func(t *testing.T) {
		dataDir := t.TempDir()
		authDir := filepath.Join(dataDir, "auth")

		// Pre-create the auth dir as read-only so WriteFile fails but MkdirAll succeeds.
		require.NoError(t, os.MkdirAll(authDir, 0o500))
		t.Cleanup(func() { _ = os.Chmod(authDir, 0o700) })

		_, err := NewJWTService(JWTServiceDeps{
			SigningKey:     "",
			AccessTokenTTL: 30 * time.Minute,
			DataDir:        dataDir,
		})
		require.Error(t, err)
	})
}
