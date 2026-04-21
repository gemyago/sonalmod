package http

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	sonalmodinternal "github.com/gemyago/sonalmod/apps/sonalmod/internal"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/middleware"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/server"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1controllers"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/handlers"
	"go.uber.org/dig"
)

// Use apigen to generate v1routes
//go:generate go run github.com/gemyago/apigen server ./v1routes.yaml ./v1routes

type V1RoutesDeps struct {
	dig.In

	*v1controllers.HealthController
	*v1controllers.AuthController

	RootHandler *handlers.RootHandler

	HTTPRouter     *server.HTTPRouter
	AuthMiddleware middleware.AuthMiddleware

	Runtime    *sonalmodinternal.Runtime
	RootLogger *slog.Logger

	// UILocation is an optional path to the directory containing pre-built UI static assets.
	// When set and the directory is readable, the backend serves index.html at GET /
	// and static assets from that directory. When empty or invalid, the server runs in API-only mode.
	UILocation string `name:"config.httpServer.uiLocation" optional:"true"`
}

func SetupV1Routes(deps V1RoutesDeps) { // coverage-ignore // Little value in testing wireup code.
	rootHandler := deps.RootHandler
	rootHandler.RegisterHealthRoutes(deps.HealthController)
	rootHandler.RegisterAuthRoutes(deps.AuthController)

	// Runtime routes — protected
	deps.HTTPRouter.Handle(
		"/api/v1/runtime/",
		deps.AuthMiddleware(http.StripPrefix("/api/v1/runtime", deps.Runtime.HTTPHandler)),
	)

	mountUIRoutes(deps.RootLogger, deps.HTTPRouter, deps.UILocation)
}

// mountUIRoutes mounts UI static file serving when uiLocation is a valid readable directory.
// If uiLocation is empty or the directory cannot be opened, the server continues in API-only mode.
func mountUIRoutes(logger *slog.Logger, router *server.HTTPRouter, uiLocation string) {
	if uiLocation == "" {
		return
	}
	if _, err := os.Stat(uiLocation); err != nil {
		logger.Warn("UI location is not accessible, running in API-only mode",
			slog.String("uiLocation", uiLocation),
			slog.Any("err", err),
		)
		return
	}
	fs := http.FileServerFS(os.DirFS(uiLocation))
	router.Handle("/", fs)
}

func Register(container *dig.Container) error {
	return errors.Join(
		v1controllers.Register(container),
		container.Invoke(SetupV1Routes),
	)
}
