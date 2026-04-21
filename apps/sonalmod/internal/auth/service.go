package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

type userStore interface {
	Create(ctx context.Context, params CreateUserParams) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetByID(ctx context.Context, id string) (*User, error)
	List(ctx context.Context) ([]User, error)
	UpdatePassword(ctx context.Context, id string, newHash string) error
}

type jwtService interface {
	GenerateAccessToken(userID, username string) (string, error)
	ValidateAccessToken(tokenStr string) (*JWTClaims, error)
}

type refreshTokenStore interface {
	Create(ctx context.Context, userID string, ttl time.Duration) (opaqueToken string, err error)
	Validate(ctx context.Context, opaqueToken string) (userID string, err error)
	Delete(ctx context.Context, opaqueToken string) error
	DeleteAllForUser(ctx context.Context, userID string) error
}

type passwordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

// ServiceDeps are the dependencies for AuthService.
type ServiceDeps struct {
	UserStore         userStore
	JWTService        jwtService
	RefreshTokenStore refreshTokenStore
	PasswordHasher    passwordHasher
	RefreshTokenTTL   time.Duration `name:"config.auth.refreshTokenTTL"`
	Logger            *slog.Logger
}

// UserInfo is a public view of a user without sensitive fields.
type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// LoginResult holds the tokens and user info returned after a successful login.
type LoginResult struct {
	AccessToken  string //nolint:gosec // field name is intentional; this is a token value, not a hardcoded secret
	RefreshToken string //nolint:gosec // field name is intentional; this is a token value, not a hardcoded secret
	User         UserInfo
}

// RefreshResult holds the tokens and user info returned after a successful token refresh.
type RefreshResult struct {
	AccessToken  string //nolint:gosec // field name is intentional; this is a token value, not a hardcoded secret
	RefreshToken string //nolint:gosec // field name is intentional; this is a token value, not a hardcoded secret
	User         UserInfo
}

// AuthService orchestrates login, token refresh, and user info retrieval.
type AuthService struct { //nolint:revive // AuthService is the established name; renaming would deviate from the plan
	deps ServiceDeps
}

// NewAuthService creates a new AuthService with the given dependencies.
func NewAuthService(deps ServiceDeps) *AuthService {
	return &AuthService{deps: deps}
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	user, err := s.deps.UserStore.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}

	ok, err := s.deps.PasswordHasher.Verify(password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	accessToken, err := s.deps.JWTService.GenerateAccessToken(user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.deps.RefreshTokenStore.Create(ctx, user.ID, s.deps.RefreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	s.deps.Logger.DebugContext(ctx, "user logged in", slog.String("userID", user.ID))

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*RefreshResult, error) {
	userID, err := s.deps.RefreshTokenStore.Validate(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidRefreshToken) {
			return nil, ErrInvalidRefreshToken
		}
		return nil, fmt.Errorf("validate refresh token: %w", err)
	}

	user, err := s.deps.UserStore.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}

	if err = s.deps.RefreshTokenStore.Delete(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("delete old refresh token: %w", err)
	}

	accessToken, err := s.deps.JWTService.GenerateAccessToken(user.ID, user.Username)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	newRefreshToken, err := s.deps.RefreshTokenStore.Create(ctx, user.ID, s.deps.RefreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("create new refresh token: %w", err)
	}

	s.deps.Logger.DebugContext(ctx, "refresh token rotated", slog.String("userID", user.ID))

	return &RefreshResult{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		User: UserInfo{
			ID:       user.ID,
			Username: user.Username,
		},
	}, nil
}

func (s *AuthService) CurrentUser(ctx context.Context, userID string) (*UserInfo, error) {
	user, err := s.deps.UserStore.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:       user.ID,
		Username: user.Username,
	}, nil
}
