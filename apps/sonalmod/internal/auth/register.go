package auth

import (
	"log/slog"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/di"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/system/ident"
	"go.uber.org/dig"
)

// userStoreDeps is the DI-aware version of UserStoreDeps for container wiring.
type userStoreDIParams struct {
	dig.In

	DataDir string `name:"config.dataDir"`
	IDGen   ident.Generator
	Logger  *slog.Logger
}

// jwtServiceDIParams is the DI-aware version of JWTServiceDeps for container wiring.
type jwtServiceDIParams struct {
	dig.In

	SigningKey     string        `name:"config.auth.jwtSigningKey"`
	AccessTokenTTL time.Duration `name:"config.auth.accessTokenTTL"`
	DataDir        string        `name:"config.dataDir"`
	Logger         *slog.Logger
}

// refreshStoreDIParams is the DI-aware version of RefreshTokenStoreDeps for container wiring.
type refreshStoreDIParams struct {
	dig.In

	DataDir string `name:"config.dataDir"`
	Logger  *slog.Logger
}

// authServiceDIParams is the DI-aware version of ServiceDeps for container wiring.
type authServiceDIParams struct {
	dig.In

	UserStore         *UserStore
	JWTService        *JWTService
	RefreshTokenStore *RefreshTokenStore
	PasswordHasher    *Argon2idHasher
	RefreshTokenTTL   time.Duration `name:"config.auth.refreshTokenTTL"`
	Logger            *slog.Logger
}

func newUserStoreFromDI(params userStoreDIParams) *UserStore {
	return NewUserStore(UserStoreDeps{
		DataDir: params.DataDir,
		IDGen:   params.IDGen,
		Logger:  params.Logger,
	})
}

func newJWTServiceFromDI(params jwtServiceDIParams) (*JWTService, error) {
	return NewJWTService(JWTServiceDeps{
		SigningKey:     params.SigningKey,
		AccessTokenTTL: params.AccessTokenTTL,
		DataDir:        params.DataDir,
		Logger:         params.Logger,
	})
}

func newRefreshTokenStoreFromDI(params refreshStoreDIParams) *RefreshTokenStore {
	return NewRefreshTokenStore(RefreshTokenStoreDeps{
		DataDir: params.DataDir,
		Logger:  params.Logger,
	})
}

func newAuthServiceFromDI(params authServiceDIParams) *AuthService {
	return NewAuthService(ServiceDeps{
		UserStore:         params.UserStore,
		JWTService:        params.JWTService,
		RefreshTokenStore: params.RefreshTokenStore,
		PasswordHasher:    params.PasswordHasher,
		RefreshTokenTTL:   params.RefreshTokenTTL,
		Logger:            params.Logger,
	})
}

// Register provides all auth components into the DI container.
func Register(container *dig.Container) error {
	return di.ProvideAll(container,
		NewArgon2idHasher,
		newUserStoreFromDI,
		newJWTServiceFromDI,
		newRefreshTokenStoreFromDI,
		newAuthServiceFromDI,
	)
}
