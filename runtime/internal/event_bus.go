package internal

import (
	"context"
	"iter"
	"sync"
)

// subscriberChanBuf is the buffer size for subscriber channels. It prevents
// the publisher goroutine from blocking when a consumer is temporarily slow.
const subscriberChanBuf = 256

// EventBus is a replay-capable broadcast mechanism for a single run's events.
// Every event published is stored in an internal buffer AND forwarded to all
// live subscriber channels. Late subscribers can replay all buffered events
// without any gap, because the buffer copy and channel registration happen
// atomically under the same lock.
type EventBus struct {
	mu          sync.Mutex
	buffer      []*SessionEvent
	subscribers map[int]chan *SessionEvent
	nextID      int
	closed      bool
	done        chan struct{}
	finalErr    error
}

// NewEventBus creates a new, open EventBus ready to accept events.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan *SessionEvent),
		done:        make(chan struct{}),
	}
}

// Publish appends the event to the replay buffer and sends it to all current
// subscribers. Publish after Close is a no-op.
func (b *EventBus) Publish(event *SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.buffer = append(b.buffer, event)
	for _, ch := range b.subscribers {
		ch <- event
	}
}

// Subscribe returns an id and a channel that receives events published from
// this point forward. It is intended for the initial caller that subscribes
// before any events are produced (so no replay is needed). The caller must
// call Unsubscribe when done to release resources.
func (b *EventBus) Subscribe() (int, <-chan *SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	id := b.nextID
	b.nextID++
	c := make(chan *SessionEvent, subscriberChanBuf)
	b.subscribers[id] = c
	return id, c
}

// Unsubscribe removes the subscriber with the given id. Subsequent events will
// not be sent to its channel.
func (b *EventBus) Unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, id)
}

// Close marks the bus as done, records the final error (may be nil), closes
// all subscriber channels, and closes the Done() channel. Any subsequent
// Publish calls are no-ops.
func (b *EventBus) Close(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	b.finalErr = err

	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = make(map[int]chan *SessionEvent)

	close(b.done)
}

// Done returns a channel that is closed when the bus is closed.
func (b *EventBus) Done() <-chan struct{} {
	return b.done
}

// replaySnapshotResult holds the output of an atomic snapshot + subscribe operation.
type replaySnapshotResult struct {
	snapshot  []*SessionEvent
	liveCh    chan *SessionEvent // nil when the bus was already closed
	closedErr error              // non-nil only when closed with an error
}

// replaySnapshot atomically copies the replay buffer and, if the bus is still
// open, registers a new live subscriber channel. The result contains the
// snapshot, the live channel (nil when already closed), and any close error.
func (b *EventBus) replaySnapshot() replaySnapshotResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	snap := make([]*SessionEvent, len(b.buffer))
	copy(snap, b.buffer)

	if b.closed {
		return replaySnapshotResult{snapshot: snap, closedErr: b.finalErr}
	}

	id := b.nextID
	b.nextID++
	ch := make(chan *SessionEvent, subscriberChanBuf)
	b.subscribers[id] = ch
	return replaySnapshotResult{snapshot: snap, liveCh: ch}
}

// removeSubscriber removes the live channel from the subscriber map.
func (b *EventBus) removeSubscriber(target chan *SessionEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, ch := range b.subscribers {
		if ch == target {
			delete(b.subscribers, id)
			return
		}
	}
}

// yieldLive drains live events from liveCh, forwarding to yield. It returns
// false if the caller should stop (ctx done or yield returned false).
func (b *EventBus) yieldLive(ctx context.Context, liveCh chan *SessionEvent, yield func(*SessionEvent, error) bool) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-liveCh:
			if !ok {
				b.mu.Lock()
				finalErr := b.finalErr
				b.mu.Unlock()
				if finalErr != nil {
					yield(nil, finalErr)
				}
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// ReplayAndSubscribe is the key entry point for late subscribers. Under a
// single lock it atomically copies the current replay buffer AND registers a
// new subscriber channel, so there is no gap between buffered and live events.
//
// The returned iterator first yields all buffered events, then yields live
// events from the subscriber channel. Iteration stops when:
//   - the bus is closed (yields the final error if non-nil, then stops)
//   - ctx is cancelled (stops without error)
func (b *EventBus) ReplayAndSubscribe(ctx context.Context) iter.Seq2[*SessionEvent, error] {
	return func(yield func(*SessionEvent, error) bool) {
		result := b.replaySnapshot()

		// Yield buffered (past) events.
		for _, ev := range result.snapshot {
			if !yield(ev, nil) {
				if result.liveCh != nil {
					b.removeSubscriber(result.liveCh)
				}
				return
			}
		}

		// Bus was already closed when we subscribed.
		if result.liveCh == nil {
			if result.closedErr != nil {
				yield(nil, result.closedErr)
			}
			return
		}

		defer b.removeSubscriber(result.liveCh)
		b.yieldLive(ctx, result.liveCh, yield)
	}
}
