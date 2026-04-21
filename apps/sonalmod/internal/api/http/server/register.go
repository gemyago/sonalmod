package server

import (
	"log/slog"
	"net/http"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/middleware"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/handlers"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/di"
	"go.uber.org/dig"
)

// authMiddlewareDIParams is the DI-aware deps struct for AuthMiddleware.
type authMiddlewareDIParams struct {
	dig.In

	JWTService *auth.JWTService
	Logger     *slog.Logger
}

func newAuthMiddlewareFromDI(params authMiddlewareDIParams) middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(middleware.AuthMiddlewareDeps{
		JWTValidator: params.JWTService,
		Logger:       params.Logger,
	})
}

func Register(container *dig.Container) error {
	return di.ProvideAll(
		container,
		NewHTTPServer,
		NewRouterMiddleware,
		NewHTTPRouter,
		NewRootHandler,
		di.ProvideImplementation[*handlers.RootHandler, http.Handler],
		newAuthMiddlewareFromDI,
	)
}
