package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
	"github.com/gemyago/sonalmod/runtime/httpapi"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware(t *testing.T) {
	fake := faker.New()
	rootLogger := telemetry.RootTestLogger()

	t.Run("valid token - next handler called with CallerIdentity in context", func(t *testing.T) {
		userID := fake.UUID().V4()
		username := fake.Internet().User()

		validator := newMockjwtValidator(t)
		mw := NewAuthMiddleware(AuthMiddlewareDeps{
			JWTValidator: validator,
			Logger:       rootLogger,
		})

		claims := &auth.JWTClaims{}
		claims.Subject = userID
		claims.Username = username

		tokenStr := fake.Lorem().Word()
		validator.EXPECT().ValidateAccessToken(tokenStr).Return(claims, nil)

		var gotCallerID httpapi.CallerIdentity
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCallerID = httpapi.CallerIdentityFromContext(r.Context())
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenStr))
		res := httptest.NewRecorder()

		mw(next).ServeHTTP(res, req)

		assert.True(t, nextCalled)
		require.NotNil(t, gotCallerID)
		assert.Equal(t, userID, gotCallerID.UserID())
	})

	t.Run("missing Authorization header - 401, next not called", func(t *testing.T) {
		validator := newMockjwtValidator(t)
		mw := NewAuthMiddleware(AuthMiddlewareDeps{
			JWTValidator: validator,
			Logger:       rootLogger,
		})

		nextCalled := false
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		res := httptest.NewRecorder()

		mw(next).ServeHTTP(res, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("malformed Authorization header (not Bearer) - 401, next not called", func(t *testing.T) {
		validator := newMockjwtValidator(t)
		mw := NewAuthMiddleware(AuthMiddlewareDeps{
			JWTValidator: validator,
			Logger:       rootLogger,
		})

		nextCalled := false
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Authorization", fake.Lorem().Word())
		res := httptest.NewRecorder()

		mw(next).ServeHTTP(res, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("Bearer prefix with no token value - 401, next not called", func(t *testing.T) {
		validator := newMockjwtValidator(t)
		mw := NewAuthMiddleware(AuthMiddlewareDeps{
			JWTValidator: validator,
			Logger:       rootLogger,
		})

		nextCalled := false
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Authorization", "Bearer")
		res := httptest.NewRecorder()

		mw(next).ServeHTTP(res, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})

	t.Run("invalid or expired token - 401, next not called", func(t *testing.T) {
		validator := newMockjwtValidator(t)
		mw := NewAuthMiddleware(AuthMiddlewareDeps{
			JWTValidator: validator,
			Logger:       rootLogger,
		})

		tokenStr := fake.Lorem().Word()
		validator.EXPECT().ValidateAccessToken(tokenStr).Return(nil, errors.New("token expired"))

		nextCalled := false
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			nextCalled = true
		})

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", tokenStr))
		res := httptest.NewRecorder()

		mw(next).ServeHTTP(res, req)

		assert.False(t, nextCalled)
		assert.Equal(t, http.StatusUnauthorized, res.Code)
	})
}
