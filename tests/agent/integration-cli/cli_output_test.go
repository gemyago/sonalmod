package main

import (
	"bytes"
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStringSeq struct {
	chunks []string
	err    error
}

func (f *fakeStringSeq) ConsumeEventsAsStringSeq(_ context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		for _, c := range f.chunks {
			if !yield(c, nil) {
				return
			}
		}
		if f.err != nil {
			yield("", f.err)
		}
	}
}

func TestStreamAgentOutput(t *testing.T) {
	t.Run("writes chunks in order", func(t *testing.T) {
		fake := faker.New()
		a := fake.Lorem().Word()
		b := fake.Lorem().Word()
		c := fake.Lorem().Word()

		var buf bytes.Buffer
		seq := &fakeStringSeq{chunks: []string{a, b, c}}

		require.NoError(t, streamAgentOutput(t.Context(), &buf, seq))
		assert.Equal(t, a+b+c, buf.String())
	})

	t.Run("propagates stream error while preserving partial output", func(t *testing.T) {
		fake := faker.New()
		prefix := fake.Lorem().Sentence(fake.IntBetween(2, 8))
		streamErr := errors.New(fake.Lorem().Sentence(4))

		var buf bytes.Buffer
		seq := &fakeStringSeq{chunks: []string{prefix}, err: streamErr}

		err := streamAgentOutput(t.Context(), &buf, seq)
		require.Error(t, err)
		require.ErrorIs(t, err, streamErr)
		assert.Equal(t, prefix, buf.String())
	})

	t.Run("propagates writer error", func(t *testing.T) {
		fake := faker.New()
		text := fake.Lorem().Word()
		werr := errors.New(fake.Lorem().Sentence(3))

		seq := &fakeStringSeq{chunks: []string{text}}
		err := streamAgentOutput(t.Context(), &failWriter{err: werr}, seq)
		require.Error(t, err)
		require.ErrorIs(t, err, werr)
	})
}

type failWriter struct {
	err error
}

func (w *failWriter) Write(_ []byte) (int, error) {
	return 0, w.err
}
