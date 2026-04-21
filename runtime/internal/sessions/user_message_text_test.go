package sessions

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestUserMessageText_joinsMultipleParts(t *testing.T) {
	t.Parallel()
	fake := faker.New()
	a := fake.Lorem().Word()
	b := fake.Lorem().Word()
	ev := &session.Event{
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{
				Role: string(genai.RoleUser),
				Parts: []*genai.Part{
					{Text: a},
					{Text: b},
				},
			},
		},
	}
	got := userMessageText(ev)
	require.Equal(t, a+" "+b, got)
}
