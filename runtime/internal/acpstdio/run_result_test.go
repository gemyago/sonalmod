package acpstdio

import (
	"encoding/json"
	"errors"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestACPStdioRunResultHelpers(t *testing.T) {
	t.Parallel()

	fake := faker.New()

	collectEvents := func(t *testing.T, result *rt.RunResult) []*rt.SessionEvent {
		t.Helper()

		events := make([]*rt.SessionEvent, 0)
		for event, err := range result.Events() {
			require.NoError(t, err)
			events = append(events, event)
		}

		return events
	}

	t.Run("new run result maps progress and final updates", func(t *testing.T) {
		t.Parallel()

		progressText := fake.Lorem().Sentence(3)
		finalText := fake.Lorem().Sentence(4)
		sessionID := fake.UUID().V4()

		result := NewRunResult(sessionID, &ExecutorResult{
			Updates: []Update{
				{
					Type:    "progress",
					Payload: json.RawMessage(`{"content":[{"text":"` + progressText + `"}]}`),
				},
				{
					Type:    "final",
					Payload: json.RawMessage(`{"message":"` + finalText + `"}`),
				},
			},
		})

		require.NotNil(t, result)
		assert.Equal(t, sessionID, result.SessionID())

		events := collectEvents(t, result)
		require.Len(t, events, 2)
		assert.True(t, events[0].Partial)
		assert.False(t, events[0].TurnComplete)
		require.NotNil(t, events[0].Content)
		assert.Equal(t, progressText, events[0].Content.Parts[0].Text)

		assert.False(t, events[1].Partial)
		assert.True(t, events[1].TurnComplete)
		require.NotNil(t, events[1].Content)
		assert.Equal(t, finalText, events[1].Content.Parts[0].Text)
	})

	t.Run("prompt result fallback preserves JSON when no text field exists", func(t *testing.T) {
		t.Parallel()

		result := NewRunResult(fake.UUID().V4(), &ExecutorResult{
			PromptResult: json.RawMessage("{\n  \"ok\": true,\n  \"count\": 2\n}"),
		})

		events := collectEvents(t, result)
		require.Len(t, events, 1)
		require.NotNil(t, events[0].Content)
		assert.Equal(t, `{"ok":true,"count":2}`, events[0].Content.Parts[0].Text)
		assert.True(t, events[0].TurnComplete)
	})

	t.Run("error update and executor failures map to stream errors", func(t *testing.T) {
		t.Parallel()

		updateEvent := mapACPStdioUpdateToSessionEvent(Update{
			Type:    "error",
			Payload: json.RawMessage(`{"text":"` + fake.Lorem().Sentence(3) + `"}`),
		})
		require.NotNil(t, updateEvent)
		assert.Equal(t, "acp-stdio-error", updateEvent.ErrorCode)
		assert.NotEmpty(t, updateEvent.ErrorMessage)

		typedErr := &LaunchError{
			Kind: LaunchErrorKindProtocol,
			Op:   fake.Lorem().Word(),
			Err:  errors.New(fake.Lorem().Sentence(4)),
		}
		typedEvent := ErrorSessionEvent(typedErr)
		assert.Equal(t, "acp-stdio-protocol", typedEvent.ErrorCode)
		assert.Contains(t, typedEvent.ErrorMessage, "ACP stdio protocol error")
		assert.Contains(t, typedEvent.ErrorMessage, typedErr.Err.Error())

		untypedEvent := ErrorSessionEvent(errors.New(fake.Lorem().Sentence(3)))
		assert.Equal(t, "acp-stdio-execution", untypedEvent.ErrorCode)
		assert.Contains(t, untypedEvent.ErrorMessage, "ACP stdio execution failed")
	})

	t.Run("nil result and content helpers fall back deterministically", func(t *testing.T) {
		t.Parallel()

		result := NewRunResult(fake.UUID().V4(), nil)
		events := collectEvents(t, result)
		require.Len(t, events, 1)
		assert.Contains(t, events[0].ErrorMessage, "returned no result")

		word := fake.Lorem().Word()
		assert.Empty(t, acpPayloadText(json.RawMessage("null")))
		assert.Equal(t, word, acpPayloadText(json.RawMessage(`"`+word+`"`)))
		assert.Equal(t, "not-json", compactJSON(json.RawMessage("not-json")))
		assert.Empty(t, firstNonEmpty("", "   "))
	})

	t.Run("message content joins trimmed parts", func(t *testing.T) {
		t.Parallel()

		text1 := fake.Lorem().Sentence(2)
		text2 := fake.Lorem().Sentence(2)
		assert.Empty(t, MessageContentText(nil))
		assert.Equal(t, text1+"\n"+text2, MessageContentText(&rt.MessageContent{
			Parts: []rt.MessagePart{
				{Text: "  " + text1 + "  "},
				{Text: "\n" + text2 + "\t"},
			},
		}))
	})
}
