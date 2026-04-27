package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProfileRunDispatcher(t *testing.T) {
	t.Run("returns error when profiles service is nil", func(t *testing.T) {
		dispatcher, err := NewProfileRunDispatcher(nil, NewMockAgentRunner(t))
		require.ErrorContains(t, err, "create profile run dispatcher")
		assert.Nil(t, dispatcher)
	})
}
