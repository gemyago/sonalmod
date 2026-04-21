package internal

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"sync"
)

// backgroundRunnerDep is the minimal interface [BackgroundRunner] requires from its underlying runner.
// It matches [agent.AgentRunner] (same method set; internal cannot import agent without an import cycle).
type backgroundRunnerDep interface {
	Run(ctx context.Context, params RunParams) (*RunResult, error)
	ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error)
	ListSessions(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)
}

// activeRun tracks a single in-progress background run.
type activeRun struct {
	cancel           context.CancelFunc
	eventBus         *EventBus
	preRunEventCount int
}

// BackgroundRunnerDeps holds the dependencies for NewBackgroundRunner.
type BackgroundRunnerDeps struct {
	Runner backgroundRunnerDep
	Logger *slog.Logger
}

// BackgroundRunner wraps an underlying runner and executes runs in background goroutines
// decoupled from the caller's context. It provides fan-out via EventBus and supports
// reconnection via ReadSession.
type BackgroundRunner struct {
	runner     backgroundRunnerDep
	logger     *slog.Logger
	mu         sync.Mutex
	activeRuns map[string]*activeRun
}

// NewBackgroundRunner creates a new BackgroundRunner.
func NewBackgroundRunner(deps BackgroundRunnerDeps) *BackgroundRunner {
	return &BackgroundRunner{
		runner:     deps.Runner,
		logger:     deps.Logger,
		activeRuns: make(map[string]*activeRun),
	}
}

// Run starts the agent run in a background goroutine with a server-scoped context (not derived
// from the caller's ctx). It returns a *RunResult whose Events() iterator reads from an EventBus,
// so the caller can stream events to the client. Cancelling the caller's ctx stops event delivery
// but does NOT cancel the background run.
//
// Returns an error if a run is already active for params.SessionID.
func (br *BackgroundRunner) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	// Record pre-run event count before starting.
	histResult, err := br.runner.ReadSession(ctx, ReadSessionParams{
		SessionID: params.SessionID,
		UserID:    params.UserID,
	})
	preRunEventCount := 0
	if err == nil {
		for _, iterErr := range histResult.Events() {
			if iterErr != nil {
				break
			}
			preRunEventCount++
		}
	}
	// Note: if ReadSession fails (e.g. session not yet created), preRunEventCount stays 0.

	bus := NewEventBus()
	bgCtx, cancel := context.WithCancel(context.Background())

	br.mu.Lock()
	if _, exists := br.activeRuns[params.SessionID]; exists {
		br.mu.Unlock()
		cancel()
		bus.Close(nil)
		return nil, fmt.Errorf("session %s already has an active run", params.SessionID)
	}
	br.activeRuns[params.SessionID] = &activeRun{
		cancel:           cancel,
		eventBus:         bus,
		preRunEventCount: preRunEventCount,
	}
	br.mu.Unlock()

	// Subscribe before launching the goroutine to ensure no events are missed.
	subID, subCh := bus.Subscribe()

	go br.runBackground(bgCtx, params, bus)

	// Build RunResult whose Events reads from the EventBus via the pre-registered subscriber.
	events := busSubscriberIterator(ctx, bus, subID, subCh)
	return NewRunResult(events, params.SessionID), nil
}

// runBackground is the goroutine that executes the underlying run and publishes events to the bus.
//
// The underlying runner (e.g. ADK Runner.Run) yields only events for the current invocation — it does
// not replay prior session history on the stream. preRunEventCount is therefore used only by
// ReadSession to splice stored history (first N events) with the bus; we publish every event from the
// underlying iterator.
func (br *BackgroundRunner) runBackground(ctx context.Context, params RunParams, bus *EventBus) {
	underlying, err := br.runner.Run(ctx, params)
	if err != nil {
		br.logger.ErrorContext(ctx, "BackgroundRunner: underlying Run failed",
			"sessionID", params.SessionID, "err", err)
		bus.Close(err)
		br.removeActiveRun(params.SessionID)
		return
	}

	var runErr error
	for ev, iterErr := range underlying.Events() {
		if iterErr != nil {
			runErr = iterErr
			break
		}
		bus.Publish(ev)
	}
	bus.Close(runErr)
	br.removeActiveRun(params.SessionID)
}

// removeActiveRun removes the active run entry for the given sessionID.
func (br *BackgroundRunner) removeActiveRun(sessionID string) {
	br.mu.Lock()
	delete(br.activeRuns, sessionID)
	br.mu.Unlock()
}

// busSubscriberIterator returns an iterator that reads from a pre-registered subscriber channel.
// The caller's ctx controls when iteration stops; cancelling ctx stops delivery to the caller
// but does not affect the background run. Unsubscribes when done.
func busSubscriberIterator(
	ctx context.Context,
	bus *EventBus,
	subID int,
	subCh <-chan *SessionEvent,
) iter.Seq2[*SessionEvent, error] {
	return func(yield func(*SessionEvent, error) bool) {
		defer bus.Unsubscribe(subID)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-subCh:
				if !ok {
					// Bus closed; check for final error.
					bus.mu.Lock()
					finalErr := bus.finalErr
					bus.mu.Unlock()
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
}

// ReadSession returns a ReadSessionResult for the given session. The Events() iterator
// yields a unified stream: pre-run history first, then (if active) current-run events
// from the EventBus (replay + live). IsActive() indicates whether a run is in progress.
func (br *BackgroundRunner) ReadSession(
	ctx context.Context,
	params ReadSessionParams,
) (*ReadSessionResult, error) {
	br.mu.Lock()
	ar := br.activeRuns[params.SessionID]
	br.mu.Unlock()

	if ar == nil {
		// Idle session: delegate to underlying runner for all history.
		return br.runner.ReadSession(ctx, params)
	}

	// Active session: read pre-run history (capped at preRunEventCount) + current-run from bus.
	histResult, err := br.runner.ReadSession(ctx, params)
	if err != nil {
		return nil, err
	}

	preRunCount := ar.preRunEventCount
	preRunEvents := materializePrefix(histResult.Events(), preRunCount)

	liveSeq := ar.eventBus.ReplayAndSubscribe(ctx)
	events := chainIters(sliceToIter(preRunEvents), liveSeq)

	return NewReadSessionResult(params.SessionID, true, events), nil
}

// ListSessions delegates to the underlying runner (read-only; no background fan-out).
func (br *BackgroundRunner) ListSessions(
	ctx context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return br.runner.ListSessions(ctx, params)
}

// Shutdown cancels all active runs.
func (br *BackgroundRunner) Shutdown() {
	br.mu.Lock()
	defer br.mu.Unlock()
	for _, ar := range br.activeRuns {
		ar.cancel()
	}
}

// materializePrefix reads at most n events from seq (stops on first error).
func materializePrefix(seq iter.Seq2[*SessionEvent, error], n int) []*SessionEvent {
	if n <= 0 {
		return nil
	}
	var out []*SessionEvent
	for ev, err := range seq {
		if err != nil {
			break
		}
		out = append(out, ev)
		if len(out) >= n {
			break
		}
	}
	return out
}

// chainIters yields all events from first, then all from second.
// Stops early if either iterator returns an error or yield returns false.
func chainIters(first, second iter.Seq2[*SessionEvent, error]) iter.Seq2[*SessionEvent, error] {
	return func(yield func(*SessionEvent, error) bool) {
		for ev, err := range first {
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
		for ev, err := range second {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				return
			}
			if !yield(ev, nil) {
				return
			}
		}
	}
}

// ErrSessionBusy is returned when Run is called on a session that already has an active run.
var ErrSessionBusy = errors.New("session already has an active run")
