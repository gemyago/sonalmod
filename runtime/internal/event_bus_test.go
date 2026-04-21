package internal

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBus(t *testing.T) {
	fake := faker.New()

	makeEvent := func() *SessionEvent {
		return &SessionEvent{
			Author:  fake.Internet().User(),
			Content: &SessionEventContent{Parts: []SessionEventPart{{Text: fake.Lorem().Sentence(3)}}},
		}
	}

	t.Run("Subscribe receives published events", func(t *testing.T) {
		bus := NewEventBus()
		_, ch := bus.Subscribe()

		ev := makeEvent()
		bus.Publish(ev)

		select {
		case got := <-ch:
			assert.Equal(t, ev, got)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event")
		}
	})

	t.Run("multiple subscribers each receive all events", func(t *testing.T) {
		bus := NewEventBus()
		_, ch1 := bus.Subscribe()
		_, ch2 := bus.Subscribe()

		ev1 := makeEvent()
		ev2 := makeEvent()
		bus.Publish(ev1)
		bus.Publish(ev2)

		for _, ch := range []<-chan *SessionEvent{ch1, ch2} {
			var got []*SessionEvent
			for range 2 {
				select {
				case e := <-ch:
					got = append(got, e)
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for event")
				}
			}
			assert.Equal(t, []*SessionEvent{ev1, ev2}, got)
		}
	})

	t.Run("Unsubscribe stops delivery to that subscriber", func(t *testing.T) {
		bus := NewEventBus()
		id, ch := bus.Subscribe()
		_, ch2 := bus.Subscribe()

		ev1 := makeEvent()
		bus.Publish(ev1)

		// Drain both channels from ev1 before unsubscribing.
		for _, drainCh := range []<-chan *SessionEvent{ch, ch2} {
			select {
			case got := <-drainCh:
				assert.Equal(t, ev1, got)
			case <-time.After(time.Second):
				t.Fatal("timed out draining ev1")
			}
		}

		bus.Unsubscribe(id)

		ev2 := makeEvent()
		bus.Publish(ev2)

		// ch should not receive ev2
		select {
		case extra := <-ch:
			t.Fatalf("unexpected event after unsubscribe: %v", extra)
		case <-time.After(50 * time.Millisecond):
			// expected — no event
		}

		// ch2 should still receive ev2
		select {
		case got := <-ch2:
			assert.Equal(t, ev2, got)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for event on ch2")
		}
	})

	t.Run("Close signals done and closes subscriber channels", func(t *testing.T) {
		bus := NewEventBus()
		_, ch := bus.Subscribe()

		bus.Close(nil)

		select {
		case <-bus.Done():
		case <-time.After(time.Second):
			t.Fatal("Done() not closed after Close()")
		}

		// subscriber channel should be closed
		_, ok := <-ch
		assert.False(t, ok, "subscriber channel should be closed")
	})

	t.Run("Publish after Close is a no-op and does not panic", func(t *testing.T) {
		bus := NewEventBus()
		bus.Close(nil)
		assert.NotPanics(t, func() {
			bus.Publish(makeEvent())
		})
	})

	t.Run("ReplayAndSubscribe late subscriber receives past events then live events", func(t *testing.T) {
		bus := NewEventBus()

		ev1 := makeEvent()
		ev2 := makeEvent()
		bus.Publish(ev1)
		bus.Publish(ev2)

		// Late subscriber
		seq := bus.ReplayAndSubscribe(t.Context())

		ev3 := makeEvent()
		ev4 := makeEvent()
		bus.Publish(ev3)
		bus.Publish(ev4)
		bus.Close(nil)

		var got []*SessionEvent
		for ev, err := range seq {
			require.NoError(t, err)
			got = append(got, ev)
		}

		assert.Equal(t, []*SessionEvent{ev1, ev2, ev3, ev4}, got)
	})

	t.Run("ReplayAndSubscribe context cancellation stops iteration", func(t *testing.T) {
		bus := NewEventBus()
		ev1 := makeEvent()
		bus.Publish(ev1)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		seq := bus.ReplayAndSubscribe(ctx)

		var got []*SessionEvent
		for ev, err := range seq {
			require.NoError(t, err)
			got = append(got, ev)
			// Cancel after first event; iterator should stop
			cancel()
		}

		assert.Len(t, got, 1)
		assert.Equal(t, ev1, got[0])
	})

	t.Run("ReplayAndSubscribe bus close after some live events ends iteration with error", func(t *testing.T) {
		bus := NewEventBus()
		closeErr := errors.New(fake.Lorem().Sentence(3))

		seq := bus.ReplayAndSubscribe(t.Context())

		ev1 := makeEvent()
		bus.Publish(ev1)
		bus.Close(closeErr)

		var got []*SessionEvent
		var gotErr error
		for ev, err := range seq {
			if err != nil {
				gotErr = err
				break
			}
			got = append(got, ev)
		}

		assert.Equal(t, []*SessionEvent{ev1}, got)
		assert.ErrorIs(t, gotErr, closeErr)
	})

	t.Run("ReplayAndSubscribe buffer copy and channel registration are atomic (no gap)", func(t *testing.T) {
		const numPublishers = 5
		const eventsPerPublisher = 20

		bus := NewEventBus()

		var wg sync.WaitGroup
		for range numPublishers {
			wg.Go(func() {
				for range eventsPerPublisher {
					bus.Publish(makeEvent())
				}
			})
		}

		// Subscribe while publishers are running
		seq := bus.ReplayAndSubscribe(t.Context())

		// Wait for all publishers then close
		wg.Wait()
		bus.Close(nil)

		var got []*SessionEvent
		for ev, err := range seq {
			require.NoError(t, err)
			got = append(got, ev)
		}

		// Must receive every event (no gaps)
		assert.Len(t, got, numPublishers*eventsPerPublisher)
		// Events slice should not contain nil entries.
		hasNil := slices.ContainsFunc(got, func(e *SessionEvent) bool { return e == nil })
		assert.False(t, hasNil, "no nil events")
	})

	t.Run("Close is idempotent (double-close does not panic)", func(t *testing.T) {
		bus := NewEventBus()
		bus.Close(nil)
		assert.NotPanics(t, func() {
			bus.Close(nil)
		})
	})

	t.Run("ReplayAndSubscribe caller stops during buffered replay cleans up live channel", func(t *testing.T) {
		bus := NewEventBus()
		ev1 := makeEvent()
		ev2 := makeEvent()
		bus.Publish(ev1)
		bus.Publish(ev2)

		// We call ReplayAndSubscribe while the bus is still open (so a live channel is registered),
		// but break out of the iterator after the first buffered event.
		seq := bus.ReplayAndSubscribe(t.Context())
		var got []*SessionEvent
		for ev, err := range seq {
			require.NoError(t, err)
			got = append(got, ev)
			break // stop after first event
		}

		assert.Equal(t, []*SessionEvent{ev1}, got)
		// Close the bus — should not deadlock or panic even though the caller left early.
		assert.NotPanics(t, func() { bus.Close(nil) })
	})

	t.Run("ReplayAndSubscribe yield returns false during live delivery stops iteration", func(t *testing.T) {
		bus := NewEventBus()

		// Subscribe first (no buffered events), so the events arrive live.
		seq := bus.ReplayAndSubscribe(t.Context())

		ev1 := makeEvent()
		ev2 := makeEvent()

		// Publish in a goroutine so we don't deadlock against the iterator.
		go func() {
			bus.Publish(ev1)
			bus.Publish(ev2)
		}()

		var got []*SessionEvent
		for ev, err := range seq {
			require.NoError(t, err)
			got = append(got, ev)
			break // stop after first live event — exercises the !yield path
		}

		assert.Equal(t, []*SessionEvent{ev1}, got)
	})

	t.Run("ReplayAndSubscribe on already-closed bus with error yields error", func(t *testing.T) {
		bus := NewEventBus()
		ev1 := makeEvent()
		bus.Publish(ev1)
		closeErr := errors.New(fake.Lorem().Sentence(3))
		bus.Close(closeErr)

		// Subscribe after close — should replay buffer then yield the error.
		seq := bus.ReplayAndSubscribe(t.Context())
		var got []*SessionEvent
		var gotErr error
		for ev, err := range seq {
			if err != nil {
				gotErr = err
				break
			}
			got = append(got, ev)
		}

		assert.Equal(t, []*SessionEvent{ev1}, got)
		assert.ErrorIs(t, gotErr, closeErr)
	})
}
