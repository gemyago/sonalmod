package internal

import (
	"context"
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func textOnlySessionContent(parts ...string) *SessionEventContent {
	out := make([]SessionEventPart, 0, len(parts))
	for _, p := range parts {
		out = append(out, SessionEventPart{Text: p})
	}
	return &SessionEventContent{Parts: out}
}

func TestRunResult(t *testing.T) {
	t.Run("ConsumeEventsAsString", func(t *testing.T) {
		fake := faker.New()

		makeFinalEvent := func(text string) *SessionEvent {
			return &SessionEvent{
				Content: textOnlySessionContent(text),
				Partial: false,
			}
		}

		makeNonFinalEvent := func(text string) *SessionEvent {
			return &SessionEvent{
				Content: textOnlySessionContent(text),
				Partial: true,
			}
		}

		t.Run("final response event yields its text", func(t *testing.T) {
			expected := fake.Lorem().Sentence(fake.IntBetween(3, 15))
			events := func(yield func(*SessionEvent, error) bool) {
				yield(makeFinalEvent(expected), nil)
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})

		t.Run("no final event falls back to concatenated text", func(t *testing.T) {
			part1 := fake.Lorem().Word()
			part2 := fake.Lorem().Word()
			part3 := fake.Lorem().Word()
			events := func(yield func(*SessionEvent, error) bool) {
				for _, p := range []string{part1, part2, part3} {
					if !yield(makeNonFinalEvent(p), nil) {
						return
					}
				}
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, part1+"\n"+part2+"\n"+part3, got)
		})

		t.Run("partial and final events prefer final answer text", func(t *testing.T) {
			partialChunk := fake.Lorem().Word()
			finalChunk := fake.Lorem().Sentence(fake.IntBetween(4, 12))
			events := func(yield func(*SessionEvent, error) bool) {
				if !yield(makeNonFinalEvent(partialChunk), nil) {
					return
				}

				yield(makeFinalEvent(finalChunk), nil)
			}

			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, finalChunk, got)
		})

		t.Run("stream error propagates", func(t *testing.T) {
			streamErr := errors.New(fake.Lorem().Sentence(4))
			events := func(yield func(*SessionEvent, error) bool) {
				yield(nil, streamErr)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context())
			require.Error(t, err)
			require.ErrorIs(t, err, streamErr)
		})

		t.Run("event with ErrorCode returns error", func(t *testing.T) {
			errorCode := fake.RandomStringWithLength(8)
			errorMessage := fake.Lorem().Sentence(4)
			ev := &SessionEvent{
				ErrorCode:    errorCode,
				ErrorMessage: errorMessage,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), errorCode)
			assert.Contains(t, err.Error(), errorMessage)
		})

		t.Run("empty stream returns error", func(t *testing.T) {
			events := func(_ func(*SessionEvent, error) bool) {
				// yields nothing
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context())
			require.Error(t, err)
		})

		t.Run("empty stream with EmptyResultBehaviorReturnEmpty returns empty string no error", func(t *testing.T) {
			events := func(_ func(*SessionEvent, error) bool) {
				// yields nothing
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context(),
				WithEmptyResultBehavior(EmptyResultBehaviorReturnEmpty))
			require.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("empty stream with EmptyResultBehaviorReturnError returns error", func(t *testing.T) {
			events := func(_ func(*SessionEvent, error) bool) {
				// yields nothing
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context(),
				WithEmptyResultBehavior(EmptyResultBehaviorReturnError))
			require.Error(t, err)
		})

		t.Run("nil event is skipped", func(t *testing.T) {
			expected := fake.Lorem().Sentence(fake.IntBetween(2, 10))
			events := func(yield func(*SessionEvent, error) bool) {
				if !yield(nil, nil) {
					return
				}
				yield(makeFinalEvent(expected), nil)
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})

		t.Run("nil content returns empty", func(t *testing.T) {
			ev := &SessionEvent{
				Content: nil,
				Partial: false,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context())
			require.Error(t, err)
		})

		t.Run("multiple text parts joined with newline", func(t *testing.T) {
			parts := []string{fake.Lorem().Word(), fake.Lorem().Word(), fake.Lorem().Word()}
			ev := &SessionEvent{
				Content: textOnlySessionContent(parts...),
				Partial: false,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, strings.Join(parts, "\n"), got)
		})

		t.Run("empty parts skipped", func(t *testing.T) {
			text := fake.Lorem().Word()
			ev := &SessionEvent{
				Content: &SessionEventContent{
					Parts: []SessionEventPart{
						{Text: ""},
						{Text: text},
						{Text: ""},
					},
				},
				Partial: false,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context())
			require.NoError(t, err)
			assert.Equal(t, text, got)
		})

		t.Run("with options uses same behavior as defaults", func(t *testing.T) {
			expected := fake.Lorem().Sentence(fake.IntBetween(3, 15))
			events := func(yield func(*SessionEvent, error) bool) {
				yield(makeFinalEvent(expected), nil)
			}
			result := NewRunResult(events, "")
			got, err := result.ConsumeEventsAsString(t.Context(),
				WithResultSelection(ResultSelectionConcatenateAll),
				WithEmptyResultBehavior(EmptyResultBehaviorReturnError),
				WithAgentName("test_agent"),
			)
			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})

		t.Run(
			"ResultSelectionLastOnly with multiple final events returns only last event text",
			func(t *testing.T) {
				first := fake.Lorem().Sentence(fake.IntBetween(3, 8))
				second := fake.Lorem().Sentence(fake.IntBetween(3, 8))
				last := fake.Lorem().Sentence(fake.IntBetween(3, 8))
				events := func(yield func(*SessionEvent, error) bool) {
					yield(makeFinalEvent(first), nil)
					yield(makeFinalEvent(second), nil)
					yield(makeFinalEvent(last), nil)
				}
				result := NewRunResult(events, "")
				got, err := result.ConsumeEventsAsString(t.Context(), WithResultSelection(ResultSelectionLastOnly))
				require.NoError(t, err)
				assert.Equal(t, last, got)
			})

		t.Run(
			"ResultSelectionLastOnly with multiple partial events returns only last partial text",
			func(t *testing.T) {
				part1 := fake.Lorem().Word()
				part2 := fake.Lorem().Word()
				part3 := fake.Lorem().Word()
				events := func(yield func(*SessionEvent, error) bool) {
					yield(makeNonFinalEvent(part1), nil)
					yield(makeNonFinalEvent(part2), nil)
					yield(makeNonFinalEvent(part3), nil)
				}
				result := NewRunResult(events, "")
				got, err := result.ConsumeEventsAsString(t.Context(), WithResultSelection(ResultSelectionLastOnly))
				require.NoError(t, err)
				assert.Equal(t, part3, got)
			})

		t.Run("event-level error with AgentName set includes agent name in error message", func(t *testing.T) {
			agentName := "sub_agent"
			errorCode := fake.RandomStringWithLength(8)
			errorMessage := fake.Lorem().Sentence(4)
			ev := &SessionEvent{
				ErrorCode:    errorCode,
				ErrorMessage: errorMessage,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context(), WithAgentName(agentName))
			require.Error(t, err)
			assert.Contains(t, err.Error(), agentName)
			assert.Contains(t, err.Error(), errorCode)
			assert.Contains(t, err.Error(), errorMessage)
		})

		t.Run("stream error with AgentName set includes agent name in error message", func(t *testing.T) {
			agentName := "sub_agent"
			streamErr := errors.New(fake.Lorem().Sentence(4))
			events := func(yield func(*SessionEvent, error) bool) {
				yield(nil, streamErr)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(t.Context(), WithAgentName(agentName))
			require.Error(t, err)
			require.ErrorIs(t, err, streamErr)
			assert.Contains(t, err.Error(), agentName)
		})

		t.Run("context cancelled returns context error", func(t *testing.T) {
			cancelCtx, cancel := context.WithCancel(t.Context())
			cancel()
			ev := &SessionEvent{
				Content: textOnlySessionContent("x"),
				Partial: false,
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(ev, nil)
			}
			result := NewRunResult(events, "")
			_, err := result.ConsumeEventsAsString(cancelCtx)
			require.ErrorIs(t, err, context.Canceled)
		})
	})

	t.Run("ConsumeEventsAsStringSeq", func(t *testing.T) {
		fake := faker.New()

		makeEvent := func(isPartial bool, parts ...string) *SessionEvent {
			return &SessionEvent{
				Content: textOnlySessionContent(parts...),
				Partial: isPartial,
			}
		}

		collectChunks := func(seq iter.Seq2[string, error]) ([]string, error) {
			chunks := make([]string, 0)
			for chunk, err := range seq {
				if err != nil {
					return chunks, err
				}
				chunks = append(chunks, chunk)
			}

			return chunks, nil
		}

		t.Run("yields partial chunks and skips duplicate final text", func(t *testing.T) {
			partialChunks := make([]string, fake.IntBetween(2, 5))
			for i := range partialChunks {
				partialChunks[i] = fake.Lorem().Word()
			}
			finalChunks := make([]string, fake.IntBetween(2, 4))
			for i := range finalChunks {
				finalChunks[i] = fake.Lorem().Word()
			}

			events := func(yield func(*SessionEvent, error) bool) {
				if !yield(makeEvent(true, partialChunks...), nil) {
					return
				}
				yield(makeEvent(false, finalChunks...), nil)
			}

			result := NewRunResult(events, "")
			chunks, err := collectChunks(result.ConsumeEventsAsStringSeq(t.Context()))
			require.NoError(t, err)
			assert.Equal(t, partialChunks, chunks)
		})

		t.Run("yields final chunks when no partial text exists", func(t *testing.T) {
			wantChunks := make([]string, fake.IntBetween(2, 5))
			for i := range wantChunks {
				wantChunks[i] = fake.Lorem().Word()
			}

			events := func(yield func(*SessionEvent, error) bool) {
				yield(makeEvent(false, wantChunks...), nil)
			}

			result := NewRunResult(events, "")
			chunks, err := collectChunks(result.ConsumeEventsAsStringSeq(t.Context()))
			require.NoError(t, err)
			assert.Equal(t, wantChunks, chunks)
		})

		t.Run("skips empty text parts and ignores nil events and nil content", func(t *testing.T) {
			chunk := fake.Lorem().Word()
			eventWithNilContent := &SessionEvent{
				Content: nil,
				Partial: true,
			}

			events := func(yield func(*SessionEvent, error) bool) {
				if !yield(nil, nil) {
					return
				}
				if !yield(eventWithNilContent, nil) {
					return
				}
				yield(makeEvent(false, "", chunk, ""), nil)
			}

			result := NewRunResult(events, "")
			chunks, err := collectChunks(result.ConsumeEventsAsStringSeq(t.Context()))
			require.NoError(t, err)
			assert.Equal(t, []string{chunk}, chunks)
		})

		t.Run("propagates stream errors after yielding prior chunks", func(t *testing.T) {
			chunk := fake.Lorem().Word()
			streamErr := errors.New(fake.Lorem().Sentence(4))

			events := func(yield func(*SessionEvent, error) bool) {
				if !yield(makeEvent(true, chunk), nil) {
					return
				}
				yield(nil, streamErr)
			}

			result := NewRunResult(events, "")
			chunks, err := collectChunks(result.ConsumeEventsAsStringSeq(t.Context()))
			require.Error(t, err)
			require.ErrorIs(t, err, streamErr)
			assert.Equal(t, []string{chunk}, chunks)
		})

		t.Run("returns context error when context is canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			events := func(yield func(*SessionEvent, error) bool) {
				yield(makeEvent(true, fake.Lorem().Word()), nil)
			}

			result := NewRunResult(events, "")
			chunks, err := collectChunks(result.ConsumeEventsAsStringSeq(ctx))
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)
			assert.Empty(t, chunks)
		})

		t.Run("stops producing when consumer stops iterating", func(t *testing.T) {
			wantChunks := make([]string, fake.IntBetween(2, 5))
			for i := range wantChunks {
				wantChunks[i] = fake.Lorem().Word()
			}
			events := func(yield func(*SessionEvent, error) bool) {
				yield(makeEvent(false, wantChunks...), nil)
			}

			result := NewRunResult(events, "")
			collected := make([]string, 0, 1)
			for chunk, err := range result.ConsumeEventsAsStringSeq(t.Context()) {
				require.NoError(t, err)
				collected = append(collected, chunk)
				break
			}

			assert.Equal(t, []string{wantChunks[0]}, collected)
		})
	})

	t.Run("Events", func(t *testing.T) {
		fake := faker.New()
		ev1 := &SessionEvent{InvocationID: fake.UUID().V4()}
		ev2 := &SessionEvent{InvocationID: fake.UUID().V4()}
		streamErr := errors.New(fake.Lorem().Sentence(4))

		makeSeq := func() iter.Seq2[*SessionEvent, error] {
			return func(yield func(*SessionEvent, error) bool) {
				if !yield(ev1, nil) {
					return
				}
				if !yield(ev2, nil) {
					return
				}
				yield(nil, streamErr)
			}
		}

		collect := func(seq iter.Seq2[*SessionEvent, error]) ([]*SessionEvent, []error) {
			var evs []*SessionEvent
			var errs []error
			for e, err := range seq {
				evs = append(evs, e)
				errs = append(errs, err)
			}
			return evs, errs
		}

		t.Run("yields same sequence as underlying iterator", func(t *testing.T) {
			wantE, wantErr := collect(makeSeq())
			gotE, gotErr := collect(NewRunResult(makeSeq(), fake.UUID().V4()).Events())
			assert.Equal(t, wantE, gotE)
			assert.Equal(t, wantErr, gotErr)
			require.Len(t, wantErr, 3)
			assert.NoError(t, wantErr[0])
			assert.NoError(t, wantErr[1])
			assert.Equal(t, streamErr, wantErr[2])
		})

		t.Run("empty sequence", func(t *testing.T) {
			makeEmpty := func() iter.Seq2[*SessionEvent, error] {
				return func(_ func(*SessionEvent, error) bool) {}
			}
			wantE, wantErr := collect(makeEmpty())
			gotE, gotErr := collect(NewRunResult(makeEmpty(), "").Events())
			assert.Equal(t, wantE, gotE)
			assert.Equal(t, wantErr, gotErr)
		})
	})

	t.Run("SessionID", func(t *testing.T) {
		fake := faker.New()
		ev := &SessionEvent{
			Content: textOnlySessionContent(fake.Lorem().Word()),
			Partial: false,
		}
		events := func(yield func(*SessionEvent, error) bool) {
			yield(ev, nil)
		}

		t.Run("returns session ID passed to NewRunResult", func(t *testing.T) {
			sessionID := fake.UUID().V4()
			result := NewRunResult(events, sessionID)
			assert.Equal(t, sessionID, result.SessionID())
		})

		t.Run("returns empty string when session ID not provided", func(t *testing.T) {
			result := NewRunResult(events, "")
			assert.Empty(t, result.SessionID())
		})
	})
}
