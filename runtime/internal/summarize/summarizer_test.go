package summarize

import (
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
)

func TestTruncatingSummarizer(t *testing.T) {
	t.Parallel()

	fake := faker.New()
	s := NewTruncatingSummarizer()

	t.Run("short text returned as-is", func(t *testing.T) {
		t.Parallel()
		in := fake.Lorem().Word()
		out, err := s.Summarize(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	})

	t.Run("empty text returns empty string", func(t *testing.T) {
		t.Parallel()
		out, err := s.Summarize(t.Context(), "")
		require.NoError(t, err)
		require.Empty(t, out)
	})

	t.Run("text exactly at 50 chars returned as-is", func(t *testing.T) {
		t.Parallel()
		ch := fake.Letter()
		in := strings.Repeat(ch, 50)
		out, err := s.Summarize(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, in, out)
	})

	t.Run("long text truncated at word boundary with ellipsis", func(t *testing.T) {
		t.Parallel()
		word := fake.Lorem().Word()
		if len(word) > 49 {
			word = word[:49]
		}
		ch := fake.Letter()
		in := word + " " + strings.Repeat(ch, 100)
		out, err := s.Summarize(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, word+"...", out)
	})

	t.Run("long text with no space in first 50 bytes uses hard truncate", func(t *testing.T) {
		t.Parallel()
		ch := fake.Letter()
		in := strings.Repeat(ch, 51)
		out, err := s.Summarize(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, strings.Repeat(ch, 50)+"...", out)
	})

	t.Run("custom max length via opts", func(t *testing.T) {
		t.Parallel()
		custom := NewTruncatingSummarizer(WithTruncatingSummarizerMaxLen(10))
		w1 := fake.Lorem().Word()
		r1 := []rune(w1)
		if len(r1) > 9 {
			w1 = string(r1[:9])
		}
		w2 := fake.Lorem().Word()
		require.NotEmpty(t, w2)
		pad := fake.Letter()
		in := w1 + " " + w2 + " " + strings.Repeat(pad, 50)
		out, err := custom.Summarize(t.Context(), in)
		require.NoError(t, err)
		require.Equal(t, w1+"...", out)
	})
}
