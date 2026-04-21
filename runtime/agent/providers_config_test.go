//go:build !release

package agent

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal"
	"github.com/stretchr/testify/require"
)

func TestNewFileProvidersConfigService(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()
	t.Run("creates service with valid base dir", func(t *testing.T) {
		svc, err := NewFileProvidersConfigService(t.TempDir(), rootTestLogger)
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}

func TestNewDatabaseProvidersConfigService(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()
	t.Run("creates service with sqlite memory dsn", func(t *testing.T) {
		svc, err := NewDatabaseProvidersConfigService(":memory:", rootTestLogger, "")
		require.NoError(t, err)
		require.NotNil(t, svc)
	})
}
