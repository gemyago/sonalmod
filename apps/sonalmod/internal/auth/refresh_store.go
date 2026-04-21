package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// ErrInvalidRefreshToken is returned when a refresh token is missing, expired,
// or otherwise invalid.
var ErrInvalidRefreshToken = errors.New("invalid or expired refresh token")

// RefreshToken is the on-disk representation of a refresh token.
type RefreshToken struct {
	UserID    string    `json:"userId"`
	TokenHash string    `json:"tokenHash"` // SHA-256 hex of the opaque token
	ExpiresAt time.Time `json:"expiresAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// RefreshTokenStoreDeps are the dependencies for RefreshTokenStore.
type RefreshTokenStoreDeps struct {
	DataDir string `name:"config.dataDir"`
	Logger  *slog.Logger
}

// RefreshTokenStore manages opaque refresh tokens backed by the filesystem.
type RefreshTokenStore struct {
	deps RefreshTokenStoreDeps
}

// NewRefreshTokenStore creates a RefreshTokenStore.
func NewRefreshTokenStore(deps RefreshTokenStoreDeps) *RefreshTokenStore {
	return &RefreshTokenStore{deps: deps}
}

func (s *RefreshTokenStore) tokensDir() string {
	return filepath.Join(s.deps.DataDir, "auth", "refresh-tokens")
}

func (s *RefreshTokenStore) tokenFilePath(hash string) string {
	return filepath.Join(s.tokensDir(), hash+".json")
}

func hashToken(opaqueToken string) string {
	sum := sha256.Sum256([]byte(opaqueToken))
	return hex.EncodeToString(sum[:])
}

// Create generates a new opaque refresh token for userID with the given TTL,
// persists its SHA-256 hash, and returns the raw opaque token to the caller.
func (s *RefreshTokenStore) Create(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate refresh token: %w", err)
	}

	opaqueToken := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(opaqueToken)

	now := time.Now().UTC()
	record := RefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}

	data, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("marshal refresh token: %w", err)
	}

	if err = os.MkdirAll(s.tokensDir(), 0o700); err != nil {
		return "", fmt.Errorf("create refresh-tokens directory: %w", err)
	}

	dest := s.tokenFilePath(hash)
	tmp := dest + ".tmp"

	if err = os.WriteFile(tmp, data, 0o600); err != nil {
		return "", fmt.Errorf("write temp refresh token file: %w", err)
	}

	if err = os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("rename refresh token file: %w", err)
	}

	if s.deps.Logger != nil {
		s.deps.Logger.DebugContext(ctx, "refresh token created", slog.String("userID", userID))
	}

	return opaqueToken, nil
}

// Validate hashes the presented token, reads the persisted record, and checks
// expiry. Returns the userID or ErrInvalidRefreshToken.
func (s *RefreshTokenStore) Validate(_ context.Context, opaqueToken string) (string, error) {
	hash := hashToken(opaqueToken)
	data, err := os.ReadFile(s.tokenFilePath(hash))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrInvalidRefreshToken
		}
		return "", fmt.Errorf("read refresh token file: %w", err)
	}

	var record RefreshToken
	if err = json.Unmarshal(data, &record); err != nil {
		return "", fmt.Errorf("unmarshal refresh token: %w", err)
	}

	if time.Now().After(record.ExpiresAt) {
		return "", ErrInvalidRefreshToken
	}

	return record.UserID, nil
}

// Delete removes the file for the given opaque token.
func (s *RefreshTokenStore) Delete(_ context.Context, opaqueToken string) error {
	hash := hashToken(opaqueToken)
	err := os.Remove(s.tokenFilePath(hash))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete refresh token file: %w", err)
	}
	return nil
}

// DeleteAllForUser scans the tokens directory and removes all tokens belonging
// to the given userID.
func (s *RefreshTokenStore) DeleteAllForUser(ctx context.Context, userID string) error {
	entries, err := os.ReadDir(s.tokensDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read refresh-tokens directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		s.deleteIfOwnedBy(ctx, entry.Name(), userID)
	}

	return nil
}

// deleteIfOwnedBy reads the token file and removes it when it belongs to userID.
// Errors are logged as warnings and do not propagate.
func (s *RefreshTokenStore) deleteIfOwnedBy(ctx context.Context, name, userID string) {
	fullPath := filepath.Join(s.tokensDir(), name)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		s.warnf(ctx, "failed to read refresh token file, skipping", name, err)
		return
	}

	var record RefreshToken
	if err = json.Unmarshal(data, &record); err != nil {
		s.warnf(ctx, "failed to unmarshal refresh token file, skipping", name, err)
		return
	}

	if record.UserID != userID {
		return
	}

	if err = os.Remove(fullPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.warnf(ctx, "failed to delete refresh token file", name, err)
	}
}

func (s *RefreshTokenStore) warnf(ctx context.Context, msg, file string, err error) {
	if s.deps.Logger != nil {
		s.deps.Logger.WarnContext(ctx, msg, slog.String("file", file), slog.Any("error", err))
	}
}
