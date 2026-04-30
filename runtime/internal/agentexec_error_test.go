//go:build !release

package internal

import (
	"errors"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentExecError(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("wraps non-nil errors", func(t *testing.T) {
		t.Parallel()

		op := fake.Lorem().Word()
		kind := AgentExecErrorKindExecution
		innerErr := errors.New(fake.Lorem().Sentence(4))

		err := WrapAgentExecError(kind, op, innerErr)
		require.Error(t, err)

		var execErr *AgentExecError
		require.ErrorAs(t, err, &execErr)
		assert.Equal(t, kind, execErr.Kind)
		assert.Equal(t, op, execErr.Op)
		assert.Equal(t, innerErr, execErr.Err)
		assert.Equal(t, "agent execution "+op+" ("+string(kind)+"): "+innerErr.Error(), execErr.Error())
		assert.ErrorIs(t, err, innerErr)
	})

	t.Run("returns nil for nil errors", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, WrapAgentExecError(AgentExecErrorKindValidation, fake.Lorem().Word(), nil))
	})
}
