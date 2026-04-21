package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/app"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
)

func NewAppErrorHandler(rootLogger *slog.Logger) func(w http.ResponseWriter, r *http.Request, err error) {
	logger := rootLogger.WithGroup("error-handler")
	return func(w http.ResponseWriter, r *http.Request, err error) {
		var errNotFound *app.NotFoundError
		var errInvalidInput *app.InvalidInputError
		var errConflict *app.ConflictError
		var errUnauthorized *app.UnauthorizedError
		logLevel := slog.LevelWarn
		switch {
		case errors.As(err, &errInvalidInput):
			w.WriteHeader(http.StatusBadRequest)
		case errors.As(err, &errConflict):
			w.WriteHeader(http.StatusConflict)
		case errors.As(err, &errNotFound):
			w.WriteHeader(http.StatusNotFound)
		case errors.As(err, &errUnauthorized),
			errors.Is(err, auth.ErrInvalidCredentials),
			errors.Is(err, auth.ErrInvalidRefreshToken),
			errors.Is(err, auth.ErrUserNotFound):
			w.WriteHeader(http.StatusUnauthorized)
		default:
			logLevel = slog.LevelError
			w.WriteHeader(http.StatusInternalServerError)
		}
		logger.Log(r.Context(), logLevel, "Failed to process request", telemetry.ErrAttr(err))
	}
}
