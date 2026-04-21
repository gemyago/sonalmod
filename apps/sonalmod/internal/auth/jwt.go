package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims is the set of claims embedded in each access token.
type JWTClaims struct {
	jwt.RegisteredClaims

	Username string `json:"username"`
}

// JWTServiceDeps are the dependencies for JWTService.
type JWTServiceDeps struct {
	// SigningKey is the HMAC-SHA256 signing key. If empty, a key is
	// auto-generated and persisted to <DataDir>/auth/jwt-signing-key.
	SigningKey     string        `name:"config.auth.jwtSigningKey"`
	AccessTokenTTL time.Duration `name:"config.auth.accessTokenTTL"`
	DataDir        string        `name:"config.dataDir"`
	Logger         *slog.Logger
}

// JWTService issues and validates JWT access tokens.
type JWTService struct {
	signingKey []byte
	ttl        time.Duration
	logger     *slog.Logger
}

// NewJWTService creates a JWTService, resolving the signing key.
func NewJWTService(deps JWTServiceDeps) (*JWTService, error) {
	key, err := resolveSigningKey(deps.SigningKey, deps.DataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve JWT signing key: %w", err)
	}

	return &JWTService{
		signingKey: key,
		ttl:        deps.AccessTokenTTL,
		logger:     deps.Logger,
	}, nil
}

// GenerateAccessToken creates a signed JWT access token for the given user.
func (s *JWTService) GenerateAccessToken(userID, username string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
		Username: username,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return signed, nil
}

// ValidateAccessToken parses and validates a JWT access token string.
func (s *JWTService) ValidateAccessToken(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.signingKey, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse access token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid access token claims")
	}

	return claims, nil
}

// resolveSigningKey returns the signing key bytes.
// Priority: explicit config value → persisted file → auto-generate and persist.
func resolveSigningKey(configKey, dataDir string) ([]byte, error) {
	if configKey != "" {
		return []byte(configKey), nil
	}

	keyFile := filepath.Join(dataDir, "auth", "jwt-signing-key")

	data, err := os.ReadFile(keyFile)
	if err == nil {
		return data, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read jwt signing key file: %w", err)
	}

	// Generate a random 256-bit key and persist it.
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return nil, fmt.Errorf("generate jwt signing key: %w", err)
	}

	encoded := []byte(hex.EncodeToString(raw))

	if err = os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return nil, fmt.Errorf("create auth directory: %w", err)
	}

	if err = os.WriteFile(keyFile, encoded, 0o600); err != nil {
		return nil, fmt.Errorf("write jwt signing key file: %w", err)
	}

	return encoded, nil
}
