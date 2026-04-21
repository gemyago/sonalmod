package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

type Argon2idHasher struct {
	memory      uint32
	time        uint32
	parallelism uint8
	saltLen     uint32
	keyLen      uint32
}

func NewArgon2idHasher() *Argon2idHasher {
	return &Argon2idHasher{
		memory:      64 * 1024, // 64 MB
		time:        1,
		parallelism: 4,
		saltLen:     16,
		keyLen:      32,
	}
}

func (h *Argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.parallelism, h.keyLen)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedKey := base64.RawStdEncoding.EncodeToString(key)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.memory,
		h.time,
		h.parallelism,
		encodedSalt,
		encodedKey,
	)

	return encoded, nil
}

func (h *Argon2idHasher) Verify(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	// Expected format: $argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>
	// Split by "$" gives: ["", "argon2id", "v=19", "m=65536,t=1,p=4", "<salt>", "<hash>"]
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format: expected 6 parts, got %d", len(parts))
	}

	if parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format: unsupported algorithm %q", parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("invalid hash format: parse version: %w", err)
	}
	if version != argon2.Version {
		return false, fmt.Errorf("invalid hash format: unsupported argon2 version %d", version)
	}

	var memory uint32
	var time uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &parallelism); err != nil {
		return false, fmt.Errorf("invalid hash format: parse parameters: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid hash format: decode salt: %w", err)
	}

	storedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid hash format: decode key: %w", err)
	}

	keyLen := uint32(len(storedKey)) //nolint:gosec // length of a decoded base64 blob is always within uint32 range
	derivedKey := argon2.IDKey([]byte(password), salt, time, memory, parallelism, keyLen)

	if subtle.ConstantTimeCompare(storedKey, derivedKey) == 1 {
		return true, nil
	}
	return false, nil
}
