//go:build !release

package agentapi

import (
	"testing"
	"time"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapListedSessionMetadata(t *testing.T) {
	t.Parallel()

	t.Run("preserves non-empty title", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		ts := time.Unix(int64(fake.IntBetween(1577836800, 1893456000)), 0).UTC()
		in := rt.SessionMetadata{
			SessionID: fake.UUID().V4(),
			Title:     fake.Lorem().Sentence(4),
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		out := mapListedSessionMetadata(in)
		assert.Equal(t, in.Title, out.Title)
		assert.Equal(t, in.SessionID, out.SessionId)
		require.True(t, out.CreatedAt.Equal(ts))
		require.True(t, out.UpdatedAt.Equal(ts))
	})

	t.Run("fallback title when empty", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		ts := time.Unix(int64(fake.IntBetween(1577836800, 1893456000)), 0).UTC()
		want := "Session " + ts.Format("Jan 2 15:04")
		in := rt.SessionMetadata{
			SessionID: "sid",
			Title:     "",
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		out := mapListedSessionMetadata(in)
		assert.Equal(t, want, out.Title)
	})
}
