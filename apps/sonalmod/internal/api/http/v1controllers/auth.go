package v1controllers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/middleware"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/handlers"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/api/http/v1routes/models"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/app"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/runtime/httpapi"
	"go.uber.org/dig"
)

// AuthenticatingService is the auth dependency for AuthController.
type AuthenticatingService interface {
	Login(ctx context.Context, username, password string) (*auth.LoginResult, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.RefreshResult, error)
	CurrentUser(ctx context.Context, userID string) (*auth.UserInfo, error)
}

var _ AuthenticatingService = (*auth.AuthService)(nil)

// AuthControllerDeps holds the dependencies for AuthController.
type AuthControllerDeps struct {
	dig.In

	AuthService    AuthenticatingService
	AuthMiddleware middleware.AuthMiddleware
}

// AuthController handles authentication HTTP routes (apigen-generated contract).
type AuthController struct {
	deps AuthControllerDeps
}

// NewAuthController creates a new AuthController.
func NewAuthController(deps AuthControllerDeps) *AuthController {
	return &AuthController{deps: deps}
}

var _ handlers.AuthController = (*AuthController)(nil)

// AuthLogin implements handlers.AuthController.
func (c *AuthController) AuthLogin(
	builder handlers.HandlerBuilder[*models.AuthLoginParams, *models.AuthSessionResponse],
) http.Handler {
	return builder.HandleWith(func(
		ctx context.Context,
		params *models.AuthLoginParams,
	) (*models.AuthSessionResponse, error) {
		result, err := c.deps.AuthService.Login(ctx, params.Payload.Username, params.Payload.Password)
		if err != nil {
			return nil, fmt.Errorf("login: %w", err)
		}

		user := models.UserInfo{ID: result.User.ID, Username: result.User.Username}
		return &models.AuthSessionResponse{
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			User:         &user,
		}, nil
	})
}

// AuthRefresh implements handlers.AuthController.
func (c *AuthController) AuthRefresh(
	builder handlers.HandlerBuilder[*models.AuthRefreshParams, *models.AuthSessionResponse],
) http.Handler {
	return builder.HandleWith(func(
		ctx context.Context,
		params *models.AuthRefreshParams,
	) (*models.AuthSessionResponse, error) {
		result, err := c.deps.AuthService.Refresh(ctx, params.Payload.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("refresh: %w", err)
		}

		user := models.UserInfo{ID: result.User.ID, Username: result.User.Username}
		return &models.AuthSessionResponse{
			AccessToken:  result.AccessToken,
			RefreshToken: result.RefreshToken,
			User:         &user,
		}, nil
	})
}

// AuthMe implements handlers.AuthController.
func (c *AuthController) AuthMe(
	builder handlers.NoParamsHandlerBuilder[*models.UserInfo],
) http.Handler {
	inner := builder.HandleWith(func(ctx context.Context) (*models.UserInfo, error) {
		identity := httpapi.CallerIdentityFromContext(ctx)
		if identity == nil {
			return nil, app.NewErrUnauthorized("unauthorized")
		}

		userInfo, err := c.deps.AuthService.CurrentUser(ctx, identity.UserID())
		if err != nil {
			return nil, fmt.Errorf("get current user: %w", err)
		}

		return &models.UserInfo{ID: userInfo.ID, Username: userInfo.Username}, nil
	})
	return c.deps.AuthMiddleware(inner)
}
