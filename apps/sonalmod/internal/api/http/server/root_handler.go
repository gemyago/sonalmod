package server

import (
	"log/slog"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/middleware"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/handlers"
	"go.uber.org/dig"
)

type RootHandlerDeps struct {
	dig.In

	RootLogger *slog.Logger
	Router     *HTTPRouter
}

func NewRootHandler(
	deps RootHandlerDeps,
) *handlers.RootHandler { // coverage-ignore // Little value in testing wireup code.
	logger := deps.RootLogger.WithGroup("http")

	rootHandler := handlers.NewRootHandler(
		deps.Router,
		handlers.WithLogger(logger),
		handlers.WithActionErrorHandler(
			middleware.NewAppErrorHandler(deps.RootLogger),
		),
	)

	return rootHandler
}
