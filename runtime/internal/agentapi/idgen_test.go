package agentapi

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultGenerator(t *testing.T) {
	t.Run("NewV7", func(t *testing.T) {
		gen := NewDefaultIDGen()
		id, err := gen.NewV7()
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, id)
		assert.EqualValues(t, 7, id.Version())
	})

	t.Run("MustNewV7", func(t *testing.T) {
		gen := NewDefaultIDGen()
		id := gen.MustNewV7()
		assert.NotEqual(t, uuid.Nil, id)
		assert.EqualValues(t, 7, id.Version())
	})

	t.Run("Uniqueness", func(t *testing.T) {
		gen := NewDefaultIDGen()
		id1 := gen.MustNewV7()
		id2 := gen.MustNewV7()
		assert.NotEqual(t, id1, id2)
	})
}

func TestMockGenerator(t *testing.T) {
	t.Run("NewV7_returns_nextGenerated_then_advances", func(t *testing.T) {
		mock := NewMockIDGen()
		next := MockIDGenNextGenerated(mock)
		id, err := mock.NewV7()
		require.NoError(t, err)
		assert.Equal(t, next, id)
		assert.Equal(t, id, MockIDGenLastGenerated(mock))
	})

	t.Run("successive_calls_yield_distinct_ids", func(t *testing.T) {
		mock := NewMockIDGen()
		a, err := mock.NewV7()
		require.NoError(t, err)
		b, err := mock.NewV7()
		require.NoError(t, err)
		assert.NotEqual(t, a, b)
	})

	t.Run("MustNewV7_matches_NewV7", func(t *testing.T) {
		mock := NewMockIDGen()
		expected := MockIDGenNextGenerated(mock)
		got := mock.MustNewV7()
		assert.Equal(t, expected, got)
	})
}
