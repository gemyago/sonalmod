package agentapi

import (
	"encoding/json"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentAPIRequestMapper(t *testing.T) {
	t.Run("ToMessageContent", func(t *testing.T) {
		mapper := NewAgentAPIRequestMapper()

		t.Run("empty_parts_nil_slice", func(t *testing.T) {
			_, err := mapper.ToMessageContent(UserMessageContent{})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidUserContent)
		})

		t.Run("empty_parts_zero_length", func(t *testing.T) {
			_, err := mapper.ToMessageContent(UserMessageContent{Parts: []UserMessagePart{}})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidUserContent)
		})

		t.Run("part_with_empty_text", func(t *testing.T) {
			_, err := mapper.ToMessageContent(UserMessageContent{
				Parts: []UserMessagePart{{Text: ""}},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidUserContent)
		})

		t.Run("part_with_whitespace_only_text", func(t *testing.T) {
			_, err := mapper.ToMessageContent(UserMessageContent{
				Parts: []UserMessagePart{{Text: "   \t\n"}},
			})
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidUserContent)
		})

		t.Run("happy_path_single_part", func(t *testing.T) {
			fake := faker.New()
			text := fake.Lorem().Sentence(4)
			got, err := mapper.ToMessageContent(UserMessageContent{
				Parts: []UserMessagePart{{Text: text}},
			})
			require.NoError(t, err)
			want := &rt.MessageContent{
				Parts: []rt.MessagePart{
					{Text: text},
				},
			}
			assert.Equal(t, want, got)
		})

		t.Run("happy_path_multiple_text_parts", func(t *testing.T) {
			fake := faker.New()
			a := fake.Lorem().Word()
			b := fake.Lorem().Word()
			got, err := mapper.ToMessageContent(UserMessageContent{
				Parts: []UserMessagePart{
					{Text: a},
					{Text: b},
				},
			})
			require.NoError(t, err)
			want := &rt.MessageContent{
				Parts: []rt.MessagePart{
					{Text: a},
					{Text: b},
				},
			}
			assert.Equal(t, want, got)
		})
	})

	t.Run("AgentRunRequestJSON", func(t *testing.T) {
		t.Run("includes_optional_profileName_and_model", func(t *testing.T) {
			fake := faker.New()
			profileName := "profile-" + fake.Lorem().Word()
			modelName := fake.Lorem().Word() + "/" + fake.Lorem().Word()
			text := fake.Lorem().Sentence(4)

			body := AgentRunRequest{
				ProfileName: &profileName,
				Model:       &modelName,
				Message: UserMessageContent{
					Parts: []UserMessagePart{
						{Text: text},
					},
				},
			}

			data, err := json.Marshal(body)
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, profileName, got["profileName"])
			assert.Equal(t, modelName, got["model"])
		})

		t.Run("omits_profileName_and_model_when_nil", func(t *testing.T) {
			fake := faker.New()
			text := fake.Lorem().Sentence(4)

			body := AgentRunRequest{
				Message: UserMessageContent{
					Parts: []UserMessagePart{
						{Text: text},
					},
				},
			}

			data, err := json.Marshal(body)
			require.NoError(t, err)

			var got map[string]any
			require.NoError(t, json.Unmarshal(data, &got))
			assert.NotContains(t, got, "profileName")
			assert.NotContains(t, got, "model")
		})
	})
}
