package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/runtime/httpapi"
)

// jwtValidator validates JWT access tokens and returns the parsed claims.
type jwtValidator interface {
	ValidateAccessToken(tokenStr string) (*auth.JWTClaims, error)
}

// AuthMiddleware is an HTTP middleware that validates JWT bearer tokens and
// injects a CallerIdentity into the request context.
type AuthMiddleware func(http.Handler) http.Handler

// AuthMiddlewareDeps holds the dependencies for NewAuthMiddleware.
type AuthMiddlewareDeps struct {
	JWTValidator jwtValidator
	Logger       *slog.Logger
}

// jwtCallerIdentity is the CallerIdentity implementation backed by a JWT claim.
type jwtCallerIdentity struct{ userID string }

func (j *jwtCallerIdentity) UserID() string { return j.userID }

// NewAuthMiddleware creates an AuthMiddleware that validates Bearer JWT tokens.
func NewAuthMiddleware(deps AuthMiddlewareDeps) AuthMiddleware {
	logger := deps.Logger.WithGroup("auth-middleware")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeUnauthorized(w, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || strings.TrimSpace(parts[1]) == "" {
				writeUnauthorized(w, "malformed authorization header")
				return
			}

			tokenStr := parts[1]
			claims, err := deps.JWTValidator.ValidateAccessToken(tokenStr)
			if err != nil {
				logger.DebugContext(r.Context(), "token validation failed", slog.String("error", err.Error()))
				writeUnauthorized(w, "invalid or expired token")
				return
			}

			identity := &jwtCallerIdentity{userID: claims.Subject}
			ctx := httpapi.ContextWithCallerIdentity(r.Context(), identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
