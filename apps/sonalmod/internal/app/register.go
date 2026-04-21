package app

import (
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/di"
	"go.uber.org/dig"
)

func Register(container *dig.Container) error {
	return di.ProvideAll(container)
}
