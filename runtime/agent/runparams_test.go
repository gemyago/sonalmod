package agent

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunParams(t *testing.T) {
	t.Run("WithText sets identity model and message", func(t *testing.T) {
		fake := faker.New()
		uid := fake.Lorem().Word()
		sid := fake.Lorem().Word()
		model := fake.Lorem().Word() + "/" + fake.Lorem().Word()
		text := fake.Lorem().Sentence(3)

		p := NewRunParams(uid, sid, model).WithText(text)
		assert.Equal(t, uid, p.UserID)
		assert.Equal(t, sid, p.SessionID)
		assert.Equal(t, model, p.Model)
		require.NotNil(t, p.Message)
		require.Len(t, p.Message.Parts, 1)
		assert.Equal(t, text, p.Message.Parts[0].Text)
	})
}
