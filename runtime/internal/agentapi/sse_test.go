package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rt "github.com/gemyago/sonalmod/runtime/internal"
	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// stubStreamMapper returns queued StreamEvents in order for each non-nil *rt.SessionEvent.
type stubStreamMapper struct {
	queue []StreamEvent
	idx   int
}

func (m *stubStreamMapper) ToStreamEvent(ev *rt.SessionEvent) (StreamEvent, error) {
	if ev == nil {
		return StreamEvent{}, ErrNilSessionEvent
	}
	if m.idx >= len(m.queue) {
		return StreamEvent{}, errors.New("stubStreamMapper: exhausted queue")
	}
	se := m.queue[m.idx]
	m.idx++
	return se, nil
}

func fakeRunResult(sessionID string, events []*session.Event) *rt.RunResult {
	seq := func(yield func(*rt.SessionEvent, error) bool) {
		for _, e := range events {
			if !yield(rt.MapADKSessionEvent(e), nil) {
				return
			}
		}
	}
	return rt.NewRunResult(seq, sessionID)
}

func parseSSEBlocks(body string) []struct {
	event string
	data  string
} {
	var blocks []struct {
		event string
		data  string
	}
	for chunk := range strings.SplitSeq(strings.TrimSpace(body), "\n\n") {
		if chunk == "" {
			continue
		}
		var evName, data string
		for line := range strings.SplitSeq(chunk, "\n") {
			switch {
			case strings.HasPrefix(line, "event:"):
				evName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			case strings.HasPrefix(line, "data:"):
				if data != "" {
					data += "\n"
				}
				data += strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			}
		}
		if evName != "" {
			blocks = append(blocks, struct {
				event string
				data  string
			}{event: evName, data: data})
		}
	}
	return blocks
}

func TestAgentAPISSEWriter(t *testing.T) {
	t.Run("StreamAgentRun", func(t *testing.T) {
		t.Run("headersOrderFlush", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessionID := fake.UUID().V4()
			invID := fake.UUID().V4()
			text := fake.Lorem().Word()

			ev := session.NewEvent(invID)
			ev.Content = &genai.Content{Parts: []*genai.Part{{Text: text}}}
			ev.Partial = true

			var agentPayload StreamEvent
			require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{
				Event: "agent",
			}))

			mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			result := fakeRunResult(sessionID, []*session.Event{ev})

			err := writer.StreamAgentRun(t.Context(), rec, result)
			require.NoError(t, err)

			ct := rec.Header().Get("Content-Type")
			assert.Contains(t, ct, "text/event-stream")
			assert.Contains(t, ct, "utf-8")
			assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))

			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 3, "sessionBound → agent → done")
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "agent", blocks[1].event)
			assert.Equal(t, "done", blocks[2].event)

			var sb SessionBoundEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sb))
			assert.Equal(t, "sessionBound", sb.Event)
			assert.Equal(t, sessionID, sb.SessionId)

			var done DoneEvent
			require.NoError(t, json.Unmarshal([]byte(blocks[2].data), &done))
			assert.Equal(t, "done", done.Event)

			assert.True(t, rec.Flushed, "each logical SSE chunk should Flush for incremental delivery")
		})

		t.Run("noAgentEvents", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			emptySess := fake.UUID().V4()

			mapper := &stubStreamMapper{queue: nil}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			result := fakeRunResult(emptySess, nil)

			err := writer.StreamAgentRun(t.Context(), rec, result)
			require.NoError(t, err)

			blocks := parseSSEBlocks(rec.Body.String())
			require.Len(t, blocks, 2)
			assert.Equal(t, "sessionBound", blocks[0].event)
			assert.Equal(t, "done", blocks[1].event)
		})

		t.Run("iteratorError", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			errIter := errors.New(fake.Lorem().Sentence(4))
			seq := func(yield func(*rt.SessionEvent, error) bool) {
				_ = yield(nil, errIter)
			}
			result := rt.NewRunResult(seq, fake.UUID().V4())

			mapper := &stubStreamMapper{queue: nil}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()

			err := writer.StreamAgentRun(t.Context(), rec, result)
			require.Error(t, err)
			require.ErrorIs(t, err, errIter)

			body := rec.Body.String()
			assert.Contains(t, body, "event: sessionBound")
			assert.Contains(t, body, "event: error")
			assert.NotContains(t, body, "event: done")
		})

		t.Run("mapperError", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			mapErr := errors.New(fake.Lorem().Sentence(3))
			ev := session.NewEvent(fake.UUID().V4())
			mapper := &stubMapperErr{err: mapErr}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			result := fakeRunResult(fake.UUID().V4(), []*session.Event{ev})

			err := writer.StreamAgentRun(t.Context(), rec, result)
			require.Error(t, err)
			require.ErrorIs(t, err, mapErr)

			body := rec.Body.String()
			assert.Contains(t, body, "event: sessionBound")
			assert.Contains(t, body, "event: error")
		})

		t.Run("nilResult", func(t *testing.T) {
			t.Parallel()

			writer := NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper())
			rec := httptest.NewRecorder()
			err := writer.StreamAgentRun(t.Context(), rec, nil)
			require.Error(t, err)
		})

		t.Run("contextCancelled_afterSessionBound", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessionID := fake.UUID().V4()
			ev := session.NewEvent(fake.UUID().V4())

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			var agentPayload StreamEvent
			require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{Event: "agent"}))
			mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			result := fakeRunResult(sessionID, []*session.Event{ev})

			err := writer.StreamAgentRun(ctx, rec, result)
			require.Error(t, err)
			require.ErrorIs(t, err, context.Canceled)

			body := rec.Body.String()
			assert.Contains(t, body, "event: sessionBound")
			assert.Contains(t, body, "event: error")
			assert.NotContains(t, body, "event: done")
		})

		t.Run("iteratorError_emptyMessage_usesStreamErrorFallback", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			seq := func(yield func(*rt.SessionEvent, error) bool) {
				_ = yield(nil, errors.New(""))
			}
			result := rt.NewRunResult(seq, fake.UUID().V4())

			mapper := &stubStreamMapper{queue: nil}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()

			err := writer.StreamAgentRun(t.Context(), rec, result)
			require.Error(t, err)

			body := rec.Body.String()
			assert.Contains(t, body, "stream error")
		})

		t.Run("writeFails_onSessionBound", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessionID := fake.UUID().V4()
			mapper := &stubStreamMapper{queue: nil}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			rw := &failAfterNWrites{ResponseWriter: rec, limit: 0}
			result := fakeRunResult(sessionID, nil)

			err := writer.StreamAgentRun(t.Context(), rw, result)
			require.Error(t, err)

			assert.Empty(t, rec.Body.String())
		})

		t.Run("writeFails_afterSessionBound", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessionID := fake.UUID().V4()
			ev := session.NewEvent(fake.UUID().V4())
			var agentPayload StreamEvent
			require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{Event: "agent"}))
			mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			rw := &failAfterNWrites{ResponseWriter: rec, limit: 1}
			result := fakeRunResult(sessionID, []*session.Event{ev})

			err := writer.StreamAgentRun(t.Context(), rw, result)
			require.Error(t, err)

			assert.Contains(t, rec.Body.String(), "event: sessionBound")
		})

		t.Run("writeFails_onDone", func(t *testing.T) {
			t.Parallel()

			fake := faker.New()
			sessionID := fake.UUID().V4()
			ev := session.NewEvent(fake.UUID().V4())
			var agentPayload StreamEvent
			require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{Event: "agent"}))
			mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
			writer := NewAgentAPISSEWriter(mapper)
			rec := httptest.NewRecorder()
			rw := &failAfterNWrites{ResponseWriter: rec, limit: 2}
			result := fakeRunResult(sessionID, []*session.Event{ev})

			err := writer.StreamAgentRun(t.Context(), rw, result)
			require.Error(t, err)

			body := rec.Body.String()
			assert.Contains(t, body, "event: sessionBound")
			assert.Contains(t, body, "event: agent")
		})
	})
}

// failAfterNWrites delegates to an [http.ResponseWriter] but fails after limit successful Write calls.
type failAfterNWrites struct {
	http.ResponseWriter

	limit int
	n     int
}

func (w *failAfterNWrites) Write(b []byte) (int, error) {
	w.n++
	if w.n > w.limit {
		return 0, errors.New("fail write")
	}
	return w.ResponseWriter.Write(b)
}

func fakeReadSessionResult(sessionID string, isActive bool, events []*session.Event) *rt.ReadSessionResult {
	seq := func(yield func(*rt.SessionEvent, error) bool) {
		for _, e := range events {
			if !yield(rt.MapADKSessionEvent(e), nil) {
				return
			}
		}
	}
	return rt.NewReadSessionResult(sessionID, isActive, seq)
}

func TestAgentAPISSEWriter_StreamSessionRead(t *testing.T) {
	t.Parallel()

	t.Run("idleSession_sessionBoundStatusAndDone", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		sessionID := fake.UUID().V4()

		mapper := &stubStreamMapper{queue: nil}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()
		output := fakeReadSessionResult(sessionID, false, nil)

		err := writer.StreamSessionRead(t.Context(), rec, output)
		require.NoError(t, err)

		assert.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")

		blocks := parseSSEBlocks(rec.Body.String())
		require.Len(t, blocks, 3, "sessionBound → sessionStatus → done")
		assert.Equal(t, "sessionBound", blocks[0].event)
		assert.Equal(t, "sessionStatus", blocks[1].event)
		assert.Equal(t, "done", blocks[2].event)

		var ss SessionStatusEvent
		require.NoError(t, json.Unmarshal([]byte(blocks[1].data), &ss))
		assert.Equal(t, Idle, ss.Status)

		var sb SessionBoundEvent
		require.NoError(t, json.Unmarshal([]byte(blocks[0].data), &sb))
		assert.Equal(t, sessionID, sb.SessionId)
	})

	t.Run("idleSession_withEvents", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		sessionID := fake.UUID().V4()
		invID := fake.UUID().V4()
		text := fake.Lorem().Word()

		ev := session.NewEvent(invID)
		ev.Content = &genai.Content{Parts: []*genai.Part{{Text: text}}}

		var agentPayload StreamEvent
		require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{Event: "agent"}))
		mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()
		output := fakeReadSessionResult(sessionID, false, []*session.Event{ev})

		err := writer.StreamSessionRead(t.Context(), rec, output)
		require.NoError(t, err)

		blocks := parseSSEBlocks(rec.Body.String())
		require.Len(t, blocks, 4, "sessionBound → sessionStatus → agent → done")
		assert.Equal(t, "sessionBound", blocks[0].event)
		assert.Equal(t, "sessionStatus", blocks[1].event)
		assert.Equal(t, "agent", blocks[2].event)
		assert.Equal(t, "done", blocks[3].event)
	})

	t.Run("activeSession_statusActive", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		sessionID := fake.UUID().V4()

		mapper := &stubStreamMapper{queue: nil}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()
		output := fakeReadSessionResult(sessionID, true, nil)

		err := writer.StreamSessionRead(t.Context(), rec, output)
		require.NoError(t, err)

		blocks := parseSSEBlocks(rec.Body.String())
		require.Len(t, blocks, 3)
		assert.Equal(t, "sessionStatus", blocks[1].event)

		var ss SessionStatusEvent
		require.NoError(t, json.Unmarshal([]byte(blocks[1].data), &ss))
		assert.Equal(t, Active, ss.Status)
	})

	t.Run("nilOutput_returnsError", func(t *testing.T) {
		t.Parallel()

		writer := NewAgentAPISSEWriter(NewAgentAPIStreamEventMapper())
		rec := httptest.NewRecorder()
		err := writer.StreamSessionRead(t.Context(), rec, nil)
		require.Error(t, err)
	})

	t.Run("iteratorError_writesStreamError", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		iterErr := errors.New(fake.Lorem().Sentence(4))
		seq := func(yield func(*rt.SessionEvent, error) bool) {
			_ = yield(nil, iterErr)
		}
		output := rt.NewReadSessionResult(fake.UUID().V4(), false, seq)

		mapper := &stubStreamMapper{queue: nil}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()

		err := writer.StreamSessionRead(t.Context(), rec, output)
		require.Error(t, err)
		require.ErrorIs(t, err, iterErr)

		body := rec.Body.String()
		assert.Contains(t, body, "event: sessionBound")
		assert.Contains(t, body, "event: sessionStatus")
		assert.Contains(t, body, "event: error")
		assert.NotContains(t, body, "event: done")
	})

	t.Run("contextCancelled_writesStreamError", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		ev := session.NewEvent(fake.UUID().V4())
		seq := func(yield func(*rt.SessionEvent, error) bool) {
			yield(rt.MapADKSessionEvent(ev), nil)
		}
		output := rt.NewReadSessionResult(fake.UUID().V4(), false, seq)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var agentPayload StreamEvent
		require.NoError(t, agentPayload.FromAgentStreamEvent(AgentStreamEvent{Event: "agent"}))
		mapper := &stubStreamMapper{queue: []StreamEvent{agentPayload}}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()

		err := writer.StreamSessionRead(ctx, rec, output)
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)

		body := rec.Body.String()
		assert.Contains(t, body, "event: sessionBound")
		assert.Contains(t, body, "event: sessionStatus")
		assert.Contains(t, body, "event: error")
		assert.NotContains(t, body, "event: done")
	})

	t.Run("mapperError_writesStreamError", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		mapErr := errors.New(fake.Lorem().Sentence(3))
		ev := session.NewEvent(fake.UUID().V4())
		seq := func(yield func(*rt.SessionEvent, error) bool) {
			yield(rt.MapADKSessionEvent(ev), nil)
		}
		output := rt.NewReadSessionResult(fake.UUID().V4(), false, seq)

		mapper := &stubMapperErr{err: mapErr}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()

		err := writer.StreamSessionRead(t.Context(), rec, output)
		require.Error(t, err)
		require.ErrorIs(t, err, mapErr)

		body := rec.Body.String()
		assert.Contains(t, body, "event: sessionBound")
		assert.Contains(t, body, "event: sessionStatus")
		assert.Contains(t, body, "event: error")
	})

	t.Run("writeFails_onSessionBound", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		mapper := &stubStreamMapper{queue: nil}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()
		rw := &failAfterNWrites{ResponseWriter: rec, limit: 0}
		output := fakeReadSessionResult(fake.UUID().V4(), false, nil)

		err := writer.StreamSessionRead(t.Context(), rw, output)
		require.Error(t, err)
		assert.Empty(t, rec.Body.String())
	})

	t.Run("writeFails_onSessionStatus", func(t *testing.T) {
		t.Parallel()

		fake := faker.New()
		mapper := &stubStreamMapper{queue: nil}
		writer := NewAgentAPISSEWriter(mapper)
		rec := httptest.NewRecorder()
		rw := &failAfterNWrites{ResponseWriter: rec, limit: 1}
		output := fakeReadSessionResult(fake.UUID().V4(), false, nil)

		err := writer.StreamSessionRead(t.Context(), rw, output)
		require.Error(t, err)
		assert.Contains(t, rec.Body.String(), "event: sessionBound")
	})
}

func TestWriteSSEEvent(t *testing.T) {
	t.Parallel()

	t.Run("marshalJSONError", func(t *testing.T) {
		t.Parallel()

		var se StreamEvent
		se.union = json.RawMessage(`{`)
		err := writeSSEEvent(httptest.NewRecorder(), se)
		require.Error(t, err)
	})

	t.Run("discriminatorError", func(t *testing.T) {
		t.Parallel()

		var se StreamEvent
		se.union = json.RawMessage(`[]`)
		err := writeSSEEvent(httptest.NewRecorder(), se)
		require.Error(t, err)
	})
}

type stubMapperErr struct {
	err error
}

func (m *stubMapperErr) ToStreamEvent(_ *rt.SessionEvent) (StreamEvent, error) {
	return StreamEvent{}, m.err
}
