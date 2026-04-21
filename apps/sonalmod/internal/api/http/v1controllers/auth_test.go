package v1controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/middleware"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/server"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/models"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/runtime/httpapi"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newAuthHTTPHandler mounts auth routes the same way as apigen RegisterAuthRoutes.
func newAuthHTTPHandler(ctrl *AuthController) http.Handler {
	return server.NewTestRootHandler().RegisterAuthRoutes(ctrl)
}

func TestAuthController(t *testing.T) {
	fake := faker.New()

	passthroughAuthMiddleware := middleware.AuthMiddleware(func(next http.Handler) http.Handler {
		return next
	})

	newController := func(svc AuthenticatingService) *AuthController {
		return NewAuthController(AuthControllerDeps{
			AuthService:    svc,
			AuthMiddleware: passthroughAuthMiddleware,
		})
	}

	t.Run("Login", func(t *testing.T) {
		t.Run("valid credentials - 200 with tokens and user", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			username := fake.Internet().User()
			password := fake.Internet().Password()
			userID := fake.UUID().V4()
			accessToken := fake.Lorem().Word()
			refreshToken := fake.Lorem().Word()

			svc.EXPECT().Login(mock.Anything, username, password).Return(&auth.LoginResult{
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				User:         auth.UserInfo{ID: userID, Username: username},
			}, nil)

			body, err := json.Marshal(map[string]string{"username": username, "password": password})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp models.AuthSessionResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, accessToken, resp.AccessToken)
			assert.Equal(t, refreshToken, resp.RefreshToken)
			require.NotNil(t, resp.User)
			assert.Equal(t, userID, resp.User.ID)
			assert.Equal(t, username, resp.User.Username)
		})

		t.Run("invalid credentials - 401", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			username := fake.Internet().User()
			password := fake.Internet().Password()

			svc.EXPECT().Login(mock.Anything, username, password).Return(nil, auth.ErrInvalidCredentials)

			body, err := json.Marshal(map[string]string{"username": username, "password": password})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, w.Body.String())
		})

		t.Run("missing username - 400", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			body, err := json.Marshal(map[string]string{"password": fake.Internet().Password()})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("missing password - 400", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			body, err := json.Marshal(map[string]string{"username": fake.Internet().User()})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("invalid JSON body - 400", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader([]byte("not-json")))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("service error - 500", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			username := fake.Internet().User()
			password := fake.Internet().Password()

			svc.EXPECT().Login(mock.Anything, username, password).Return(nil, errors.New("unexpected error"))

			body, err := json.Marshal(map[string]string{"username": username, "password": password})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("Refresh", func(t *testing.T) {
		t.Run("valid token - 200 with new tokens", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			oldRefreshToken := fake.Lorem().Word()
			newAccessToken := fake.Lorem().Word()
			newRefreshToken := fake.Lorem().Word()
			userID := fake.UUID().V4()
			username := fake.Internet().User()

			svc.EXPECT().Refresh(mock.Anything, oldRefreshToken).Return(&auth.RefreshResult{
				AccessToken:  newAccessToken,
				RefreshToken: newRefreshToken,
				User:         auth.UserInfo{ID: userID, Username: username},
			}, nil)

			body, err := json.Marshal(map[string]string{"refreshToken": oldRefreshToken})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp models.AuthSessionResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, newAccessToken, resp.AccessToken)
			assert.Equal(t, newRefreshToken, resp.RefreshToken)
			require.NotNil(t, resp.User)
			assert.Equal(t, userID, resp.User.ID)
			assert.Equal(t, username, resp.User.Username)
		})

		t.Run("invalid token - 401", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			refreshToken := fake.Lorem().Word()

			svc.EXPECT().Refresh(mock.Anything, refreshToken).Return(nil, auth.ErrInvalidRefreshToken)

			body, err := json.Marshal(map[string]string{"refreshToken": refreshToken})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Empty(t, w.Body.String())
		})

		t.Run("missing refreshToken - 400", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			body, err := json.Marshal(map[string]string{})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("invalid JSON body - 400", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader([]byte("not-json")))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})

		t.Run("service error - 500", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			refreshToken := fake.Lorem().Word()

			svc.EXPECT().Refresh(mock.Anything, refreshToken).Return(nil, errors.New("unexpected error"))

			body, err := json.Marshal(map[string]string{"refreshToken": refreshToken})
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})

	t.Run("Me", func(t *testing.T) {
		t.Run("with CallerIdentity - 200 with user info", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			userID := fake.UUID().V4()
			username := fake.Internet().User()

			svc.EXPECT().CurrentUser(mock.Anything, userID).Return(&auth.UserInfo{
				ID:       userID,
				Username: username,
			}, nil)

			ctx := httpapi.ContextWithCallerIdentity(t.Context(), &testCallerIdentity{userID: userID})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", http.NoBody)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			var resp models.UserInfo
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, userID, resp.ID)
			assert.Equal(t, username, resp.Username)
		})

		t.Run("without CallerIdentity - 401", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", http.NoBody)
			req = req.WithContext(t.Context())
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("user not found - 401", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			userID := fake.UUID().V4()

			svc.EXPECT().CurrentUser(mock.Anything, userID).Return(nil, auth.ErrUserNotFound)

			ctx := httpapi.ContextWithCallerIdentity(t.Context(), &testCallerIdentity{userID: userID})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", http.NoBody)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
		})

		t.Run("service error - 500", func(t *testing.T) {
			svc := NewMockAuthenticatingService(t)
			ctrl := newController(svc)

			userID := fake.UUID().V4()

			svc.EXPECT().CurrentUser(mock.Anything, userID).Return(nil, errors.New("unexpected error"))

			ctx := httpapi.ContextWithCallerIdentity(t.Context(), &testCallerIdentity{userID: userID})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", http.NoBody)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()

			newAuthHTTPHandler(ctrl).ServeHTTP(w, req)

			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})
}

// testCallerIdentity is a simple CallerIdentity implementation for testing.
type testCallerIdentity struct {
	userID string
}

func (c *testCallerIdentity) UserID() string { return c.userID }
