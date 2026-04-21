//go:build !release

package internal

import (
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestSessionEventContentFromGenAI(t *testing.T) {
	fake := faker.New()

	t.Run("nil content returns nil", func(t *testing.T) {
		got := sessionEventContentFromGenAI(nil)
		require.Nil(t, got)
	})

	t.Run("text part maps to SessionEventPart.Text", func(t *testing.T) {
		text := fake.Lorem().Sentence(5)
		c := &genai.Content{
			Role:  string(genai.RoleModel),
			Parts: []*genai.Part{{Text: text}},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		assert.Equal(t, string(genai.RoleModel), got.Role)
		require.Len(t, got.Parts, 1)
		assert.Equal(t, SessionEventPart{Text: text}, got.Parts[0])
	})

	t.Run("FunctionCall part maps to SessionEventPart.FunctionCall", func(t *testing.T) {
		callID := fake.UUID().V4()
		funcName := fake.Lorem().Word()
		argKey := fake.Lorem().Word()
		argVal := fake.IntBetween(1, 100)
		c := &genai.Content{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{
					FunctionCall: &genai.FunctionCall{
						ID:   callID,
						Name: funcName,
						Args: map[string]any{argKey: argVal},
					},
				},
			},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		require.Len(t, got.Parts, 1)
		require.NotNil(t, got.Parts[0].FunctionCall)
		assert.Nil(t, got.Parts[0].FunctionResponse)
		assert.Equal(t, callID, got.Parts[0].FunctionCall.ID)
		assert.Equal(t, funcName, got.Parts[0].FunctionCall.Name)
		assert.Equal(t, map[string]any{argKey: argVal}, got.Parts[0].FunctionCall.Args)
	})

	t.Run("FunctionResponse part maps to SessionEventPart.FunctionResponse", func(t *testing.T) {
		callID := fake.UUID().V4()
		funcName := fake.Lorem().Word()
		respKey := fake.Lorem().Word()
		respVal := fake.Lorem().Word()
		c := &genai.Content{
			Role: string(genai.RoleUser),
			Parts: []*genai.Part{
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       callID,
						Name:     funcName,
						Response: map[string]any{respKey: respVal},
					},
				},
			},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		require.Len(t, got.Parts, 1)
		require.NotNil(t, got.Parts[0].FunctionResponse)
		assert.Nil(t, got.Parts[0].FunctionCall)
		assert.Equal(t, callID, got.Parts[0].FunctionResponse.ID)
		assert.Equal(t, funcName, got.Parts[0].FunctionResponse.Name)
		assert.Equal(t, map[string]any{respKey: respVal}, got.Parts[0].FunctionResponse.Response)
	})

	t.Run("mixed text FunctionCall FunctionResponse maps all parts", func(t *testing.T) {
		text := fake.Lorem().Sentence(3)
		callID := fake.UUID().V4()
		funcName := fake.Lorem().Word()
		argKey := fake.Lorem().Word()
		argVal := fake.IntBetween(1, 100)
		respKey := fake.Lorem().Word()
		respVal := fake.Lorem().Word()
		c := &genai.Content{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{Text: text},
				{
					FunctionCall: &genai.FunctionCall{
						ID:   callID,
						Name: funcName,
						Args: map[string]any{argKey: argVal},
					},
				},
				{
					FunctionResponse: &genai.FunctionResponse{
						ID:       callID,
						Name:     funcName,
						Response: map[string]any{respKey: respVal},
					},
				},
			},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		require.Len(t, got.Parts, 3)
		assert.Equal(t, text, got.Parts[0].Text)
		assert.Nil(t, got.Parts[0].FunctionCall)
		assert.Nil(t, got.Parts[0].FunctionResponse)
		require.NotNil(t, got.Parts[1].FunctionCall)
		assert.Equal(t, callID, got.Parts[1].FunctionCall.ID)
		assert.Equal(t, funcName, got.Parts[1].FunctionCall.Name)
		require.NotNil(t, got.Parts[2].FunctionResponse)
		assert.Equal(t, callID, got.Parts[2].FunctionResponse.ID)
		assert.Equal(t, funcName, got.Parts[2].FunctionResponse.Name)
	})

	t.Run("nil part entries are skipped", func(t *testing.T) {
		text := fake.Lorem().Sentence(2)
		c := &genai.Content{
			Role:  string(genai.RoleModel),
			Parts: []*genai.Part{nil, {Text: text}, nil},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		require.Len(t, got.Parts, 1)
		assert.Equal(t, text, got.Parts[0].Text)
	})

	t.Run("empty part with no text no FunctionCall no FunctionResponse is skipped", func(t *testing.T) {
		c := &genai.Content{
			Role:  string(genai.RoleModel),
			Parts: []*genai.Part{{}},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		assert.Empty(t, got.Parts)
	})

	t.Run("multiple FunctionCall parts all mapped", func(t *testing.T) {
		call1ID := fake.UUID().V4()
		call1Name := fake.Lorem().Word()
		call2ID := fake.UUID().V4()
		call2Name := fake.Lorem().Word()
		c := &genai.Content{
			Role: string(genai.RoleModel),
			Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: call1ID, Name: call1Name, Args: map[string]any{}}},
				{FunctionCall: &genai.FunctionCall{ID: call2ID, Name: call2Name, Args: map[string]any{}}},
			},
		}

		got := sessionEventContentFromGenAI(c)

		require.NotNil(t, got)
		require.Len(t, got.Parts, 2)
		require.NotNil(t, got.Parts[0].FunctionCall)
		assert.Equal(t, call1ID, got.Parts[0].FunctionCall.ID)
		assert.Equal(t, call1Name, got.Parts[0].FunctionCall.Name)
		require.NotNil(t, got.Parts[1].FunctionCall)
		assert.Equal(t, call2ID, got.Parts[1].FunctionCall.ID)
		assert.Equal(t, call2Name, got.Parts[1].FunctionCall.Name)
	})
}
