package v1controllers

import (
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/auth"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/di"
	"go.uber.org/dig"
)

func Register(container *dig.Container) error {
	return di.ProvideAll(container,
		di.ProvideValue(&HealthController{}),
		di.ProvideImplementation[*auth.AuthService, AuthenticatingService],
		NewAuthController,
	)
}
