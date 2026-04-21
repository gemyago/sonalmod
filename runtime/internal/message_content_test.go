//go:build !release

package internal

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestMessageContentToGenAI(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, messageContentToGenAI(nil))
	})

	t.Run("preserves_part_order_and_sets_user_role", func(t *testing.T) {
		fake := faker.New()
		a := fake.Lorem().Word()
		b := fake.Lorem().Word()
		in := &MessageContent{
			Parts: []MessagePart{
				{Text: a},
				{Text: b},
			},
		}
		got := messageContentToGenAI(in)
		require.NotNil(t, got)
		assert.Equal(t, "user", got.Role)
		want := &genai.Content{
			Role: "user",
			Parts: []*genai.Part{
				{Text: a},
				{Text: b},
			},
		}
		assert.Equal(t, want, got)
	})
}
