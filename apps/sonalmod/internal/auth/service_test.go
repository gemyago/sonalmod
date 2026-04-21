package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/telemetry"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthService(t *testing.T) {
	fake := faker.New()

	makeDeps := func(t *testing.T) ServiceDeps {
		t.Helper()
		return ServiceDeps{
			UserStore:         newMockuserStore(t),
			JWTService:        newMockjwtService(t),
			RefreshTokenStore: newMockrefreshTokenStore(t),
			PasswordHasher:    newMockpasswordHasher(t),
			RefreshTokenTTL:   30 * 24 * time.Hour,
			Logger:            telemetry.RootTestLogger(),
		}
	}

	makeUser := func() *User {
		return &User{
			ID:           fake.UUID().V4(),
			Username:     fake.Internet().User(),
			PasswordHash: fake.Lorem().Text(60),
		}
	}

	t.Run("Login", func(t *testing.T) {
		t.Run("returns tokens and user info on valid credentials", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			password := fake.Internet().Password()
			accessToken := fake.Lorem().Text(40)
			refreshToken := fake.Lorem().Text(40)

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), user.Username).
				Return(user, nil)
			deps.PasswordHasher.(*mockpasswordHasher).EXPECT().
				Verify(password, user.PasswordHash).
				Return(true, nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return(accessToken, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Create(t.Context(), user.ID, deps.RefreshTokenTTL).
				Return(refreshToken, nil)

			result, err := svc.Login(t.Context(), user.Username, password)

			require.NoError(t, err)
			assert.Equal(t, &LoginResult{
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				User: UserInfo{
					ID:       user.ID,
					Username: user.Username,
				},
			}, result)
		})

		t.Run("returns ErrInvalidCredentials on wrong password", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			password := fake.Internet().Password()

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), user.Username).
				Return(user, nil)
			deps.PasswordHasher.(*mockpasswordHasher).EXPECT().
				Verify(password, user.PasswordHash).
				Return(false, nil)

			_, err := svc.Login(t.Context(), user.Username, password)

			require.ErrorIs(t, err, ErrInvalidCredentials)
		})

		t.Run("returns ErrInvalidCredentials on non-existent username", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			username := fake.Internet().User()
			password := fake.Internet().Password()

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), username).
				Return(nil, ErrUserNotFound)

			_, err := svc.Login(t.Context(), username, password)

			require.ErrorIs(t, err, ErrInvalidCredentials)
		})

		t.Run("propagates unexpected user store errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			username := fake.Internet().User()
			password := fake.Internet().Password()
			unexpectedErr := fake.Lorem().Text(20)

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), username).
				Return(nil, errors.New(unexpectedErr))

			_, err := svc.Login(t.Context(), username, password)

			require.Error(t, err)
			assert.ErrorContains(t, err, "get user by username")
		})

		t.Run("propagates password verify errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			password := fake.Internet().Password()
			verifyErr := errors.New("verify error")

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), user.Username).
				Return(user, nil)
			deps.PasswordHasher.(*mockpasswordHasher).EXPECT().
				Verify(password, user.PasswordHash).
				Return(false, verifyErr)

			_, err := svc.Login(t.Context(), user.Username, password)

			require.Error(t, err)
			assert.ErrorContains(t, err, "verify password")
		})

		t.Run("propagates JWT generation errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			password := fake.Internet().Password()
			jwtErr := errors.New("jwt error")

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), user.Username).
				Return(user, nil)
			deps.PasswordHasher.(*mockpasswordHasher).EXPECT().
				Verify(password, user.PasswordHash).
				Return(true, nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return("", jwtErr)

			_, err := svc.Login(t.Context(), user.Username, password)

			require.Error(t, err)
			assert.ErrorContains(t, err, "generate access token")
		})

		t.Run("propagates refresh token create errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			password := fake.Internet().Password()
			accessToken := fake.Lorem().Text(40)
			createErr := errors.New("create error")

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByUsername(t.Context(), user.Username).
				Return(user, nil)
			deps.PasswordHasher.(*mockpasswordHasher).EXPECT().
				Verify(password, user.PasswordHash).
				Return(true, nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return(accessToken, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Create(t.Context(), user.ID, deps.RefreshTokenTTL).
				Return("", createErr)

			_, err := svc.Login(t.Context(), user.Username, password)

			require.Error(t, err)
			assert.ErrorContains(t, err, "create refresh token")
		})
	})

	t.Run("Refresh", func(t *testing.T) {
		t.Run("returns new tokens and user info on valid refresh token", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			oldRefreshToken := fake.Lorem().Text(40)
			newAccessToken := fake.Lorem().Text(40)
			newRefreshToken := fake.Lorem().Text(40)

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), oldRefreshToken).
				Return(user.ID, nil)
			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(user, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Delete(t.Context(), oldRefreshToken).
				Return(nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return(newAccessToken, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Create(t.Context(), user.ID, deps.RefreshTokenTTL).
				Return(newRefreshToken, nil)

			result, err := svc.Refresh(t.Context(), oldRefreshToken)

			require.NoError(t, err)
			assert.Equal(t, &RefreshResult{
				AccessToken:  newAccessToken,
				RefreshToken: newRefreshToken,
				User: UserInfo{
					ID:       user.ID,
					Username: user.Username,
				},
			}, result)
		})

		t.Run("returns ErrInvalidRefreshToken on invalid token", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			invalidToken := fake.Lorem().Text(40)

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), invalidToken).
				Return("", ErrInvalidRefreshToken)

			_, err := svc.Refresh(t.Context(), invalidToken)

			require.ErrorIs(t, err, ErrInvalidRefreshToken)
		})

		t.Run("propagates unexpected validate errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			token := fake.Lorem().Text(40)
			validateErr := errors.New("validate error")

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), token).
				Return("", validateErr)

			_, err := svc.Refresh(t.Context(), token)

			require.Error(t, err)
			assert.ErrorContains(t, err, "validate refresh token")
		})

		t.Run("propagates user store errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			token := fake.Lorem().Text(40)
			getUserErr := errors.New("get user error")

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), token).
				Return(user.ID, nil)
			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(nil, getUserErr)

			_, err := svc.Refresh(t.Context(), token)

			require.Error(t, err)
			assert.ErrorContains(t, err, "get user by id")
		})

		t.Run("propagates delete old token errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			token := fake.Lorem().Text(40)
			deleteErr := errors.New("delete error")

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), token).
				Return(user.ID, nil)
			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(user, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Delete(t.Context(), token).
				Return(deleteErr)

			_, err := svc.Refresh(t.Context(), token)

			require.Error(t, err)
			assert.ErrorContains(t, err, "delete old refresh token")
		})

		t.Run("propagates JWT generation errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			token := fake.Lorem().Text(40)
			jwtErr := errors.New("jwt error")

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), token).
				Return(user.ID, nil)
			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(user, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Delete(t.Context(), token).
				Return(nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return("", jwtErr)

			_, err := svc.Refresh(t.Context(), token)

			require.Error(t, err)
			assert.ErrorContains(t, err, "generate access token")
		})

		t.Run("propagates new refresh token create errors", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()
			token := fake.Lorem().Text(40)
			newAccessToken := fake.Lorem().Text(40)
			createErr := errors.New("create error")

			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Validate(t.Context(), token).
				Return(user.ID, nil)
			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(user, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Delete(t.Context(), token).
				Return(nil)
			deps.JWTService.(*mockjwtService).EXPECT().
				GenerateAccessToken(user.ID, user.Username).
				Return(newAccessToken, nil)
			deps.RefreshTokenStore.(*mockrefreshTokenStore).EXPECT().
				Create(t.Context(), user.ID, deps.RefreshTokenTTL).
				Return("", createErr)

			_, err := svc.Refresh(t.Context(), token)

			require.Error(t, err)
			assert.ErrorContains(t, err, "create new refresh token")
		})
	})

	t.Run("CurrentUser", func(t *testing.T) {
		t.Run("returns user info for existing user", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			user := makeUser()

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), user.ID).
				Return(user, nil)

			result, err := svc.CurrentUser(t.Context(), user.ID)

			require.NoError(t, err)
			assert.Equal(t, &UserInfo{
				ID:       user.ID,
				Username: user.Username,
			}, result)
		})

		t.Run("returns ErrUserNotFound for non-existent user", func(t *testing.T) {
			deps := makeDeps(t)
			svc := NewAuthService(deps)

			userID := fake.UUID().V4()

			deps.UserStore.(*mockuserStore).EXPECT().
				GetByID(t.Context(), userID).
				Return(nil, ErrUserNotFound)

			_, err := svc.CurrentUser(t.Context(), userID)

			require.ErrorIs(t, err, ErrUserNotFound)
		})
	})
}
