package auth

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgon2idHasher(t *testing.T) {
	fake := faker.New()

	t.Run("Hash", func(t *testing.T) {
		hasher := NewArgon2idHasher()

		t.Run("produces non-empty string", func(t *testing.T) {
			password := fake.Internet().Password()
			hash, err := hasher.Hash(password)
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
		})

		t.Run("different calls produce different hashes", func(t *testing.T) {
			password := fake.Internet().Password()
			hash1, err := hasher.Hash(password)
			require.NoError(t, err)
			hash2, err := hasher.Hash(password)
			require.NoError(t, err)
			assert.NotEqual(t, hash1, hash2, "same password should produce different hashes due to random salt")
		})

		t.Run("produces argon2id encoded format", func(t *testing.T) {
			password := fake.Internet().Password()
			hash, err := hasher.Hash(password)
			require.NoError(t, err)
			assert.Contains(t, hash, "$argon2id$")
		})
	})

	t.Run("Verify", func(t *testing.T) {
		hasher := NewArgon2idHasher()

		t.Run("returns true for correct password", func(t *testing.T) {
			password := fake.Internet().Password()
			hash, err := hasher.Hash(password)
			require.NoError(t, err)
			ok, err := hasher.Verify(password, hash)
			require.NoError(t, err)
			assert.True(t, ok)
		})

		t.Run("returns false for incorrect password", func(t *testing.T) {
			password := fake.Internet().Password()
			wrongPassword := fake.Internet().Password()
			hash, err := hasher.Hash(password)
			require.NoError(t, err)
			ok, err := hasher.Verify(wrongPassword, hash)
			require.NoError(t, err)
			assert.False(t, ok)
		})

		t.Run("handles malformed hash string gracefully", func(t *testing.T) {
			t.Run("empty hash", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("random string", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, fake.Lorem().Text(50))
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("truncated argon2id hash", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$v=19$m=65536,t=1,p=4$")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("unsupported algorithm", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2i$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWtleQ")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("invalid version format", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$version=bad$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWtleQ")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("unsupported argon2 version", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$v=18$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWtleQ")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("invalid params format", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$v=19$invalid$c29tZXNhbHQ$c29tZWtleQ")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("invalid base64 salt", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$v=19$m=65536,t=1,p=4$!!!invalid!!!$c29tZWtleQ")
				require.Error(t, err)
				assert.False(t, ok)
			})

			t.Run("invalid base64 key", func(t *testing.T) {
				password := fake.Internet().Password()
				ok, err := hasher.Verify(password, "$argon2id$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$!!!invalid!!!")
				require.Error(t, err)
				assert.False(t, ok)
			})
		})
	})
}
