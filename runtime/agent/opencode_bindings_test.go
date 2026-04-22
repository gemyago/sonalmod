//go:build !release

package agent

import (
	"testing"

	"github.com/gemyago/sonalmod/runtime/internal"
	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBindingsAliases(t *testing.T) {
	require.ErrorIs(t, ErrOpenCodeBindingNotFound, cl.ErrOpenCodeBindingNotFound)
	require.ErrorIs(t, ErrOpenCodeBindingNameConflict, cl.ErrOpenCodeBindingNameConflict)
}

func TestNewOpenCodeBindingsServices(t *testing.T) {
	rootTestLogger := internal.RootTestLogger()

	fileSvc, err := NewFileOpenCodeBindingService(t.TempDir(), rootTestLogger)
	require.NoError(t, err)
	require.NotNil(t, fileSvc)

	dbSvc, err := NewDatabaseOpenCodeBindingService(":memory:", rootTestLogger, "")
	require.NoError(t, err)
	require.NotNil(t, dbSvc)
}
