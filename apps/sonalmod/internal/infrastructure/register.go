package infrastructure

import (
	"context"

	"github.com/gemyago/sonalmod/apps/sonalmod/internal/di"
	"github.com/gemyago/sonalmod/apps/sonalmod/internal/infrastructure/httpclient"
	"go.uber.org/dig"
)

func Register(_ context.Context, container *dig.Container) error {
	return di.ProvideAll(container,
		httpclient.NewClientFactory,
	)
}
