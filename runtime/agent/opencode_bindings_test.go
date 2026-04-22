//go:build !release

package agent

import (
	"testing"

	cl "github.com/gemyago/sonalmod/runtime/internal/codinglane"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeBindingsAliases(t *testing.T) {
	require.ErrorIs(t, ErrOpenCodeBindingNotFound, cl.ErrOpenCodeBindingNotFound)
	require.ErrorIs(t, ErrOpenCodeBindingNameConflict, cl.ErrOpenCodeBindingNameConflict)
}

func TestOpenCodeBindingConstructorsAreExported(t *testing.T) {
	require.NotNil(t, NewFileOpenCodeBindingService)
	require.NotNil(t, NewDatabaseOpenCodeBindingService)
}
