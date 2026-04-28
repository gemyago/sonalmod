package profilerun

import (
	"errors"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	t.Run("wraps non-nil errors", func(t *testing.T) {
		t.Parallel()

		op := fake.Lorem().Word()
		kind := ErrorKindExecution
		innerErr := errors.New(fake.Lorem().Sentence(4))

		err := WrapError(kind, op, innerErr)
		require.Error(t, err)

		var profileErr *Error
		require.ErrorAs(t, err, &profileErr)
		assert.Equal(t, kind, profileErr.Kind)
		assert.Equal(t, op, profileErr.Op)
		assert.Equal(t, innerErr, profileErr.Err)
		assert.Equal(t, "profile execution "+op+" ("+string(kind)+"): "+innerErr.Error(), profileErr.Error())
		assert.ErrorIs(t, err, innerErr)
	})

	t.Run("returns nil for nil errors", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, WrapError(ErrorKindValidation, fake.Lorem().Word(), nil))
	})
}
