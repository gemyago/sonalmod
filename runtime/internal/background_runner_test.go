//go:build !release

package internal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jaswdr/faker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundRunner(t *testing.T) {
	fake := faker.New()

	makeEvent := func(text string) *SessionEvent {
		return &SessionEvent{
			Author:  fake.Internet().User(),
			Content: &SessionEventContent{Parts: []SessionEventPart{{Text: text}}},
		}
	}

	makeRunParams := func() RunParams {
		return RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		}
	}

	makeReadSessionParams := func(sessionID, userID string) ReadSessionParams {
		return ReadSessionParams{
			SessionID: sessionID,
			UserID:    userID,
		}
	}

	// makeBlockingRunner returns a mock runner whose Run blocks until proceed is closed,
	// then yields the provided events and closes.
	makeBlockingRunner := func(
		t *testing.T,
		history []*SessionEvent,
		runEvents []*SessionEvent,
	) (*mockBackgroundRunnerDep, mockRunnerControl) {
		t.Helper()
		ctrl := mockRunnerControl{
			runStarted:  make(chan struct{}),
			runProceed:  make(chan struct{}),
			runFinished: make(chan struct{}),
		}

		dep := newMockBackgroundRunnerDep(t, history, runEvents, ctrl)
		return dep, ctrl
	}

	t.Run("Run returns RunResult whose Events yields events from underlying runner", func(t *testing.T) {
		text := fake.Lorem().Sentence(3)
		ev := makeEvent(text)
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{ev})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, params.SessionID, result.SessionID())

		close(ctrl.runProceed)

		var got []*SessionEvent
		for ev, err := range result.Events() {
			require.NoError(t, err)
			got = append(got, ev)
		}

		assert.Equal(t, []*SessionEvent{ev}, got)

		select {
		case <-ctrl.runFinished:
		case <-time.After(2 * time.Second):
			t.Fatal("background run did not finish")
		}
	})

	t.Run("ListSessions delegates to underlying runner", func(t *testing.T) {
		want := &ListSessionMetadataResult{Total: 3}
		dep := &listSessionsRecordingDep{result: want}
		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})
		ctx := t.Context()
		params := ListSessionMetadataParams{
			UserID: fake.UUID().V4(),
			Limit:  5,
			Offset: 1,
		}
		got, err := br.ListSessions(ctx, params)
		require.NoError(t, err)
		assert.Equal(t, want, got)
		assert.True(t, dep.called)
		assert.Equal(t, params, dep.gotParams)
	})

	t.Run("Run executes in background — caller ctx cancellation does not stop the run", func(t *testing.T) {
		text := fake.Lorem().Sentence(3)
		ev := makeEvent(text)
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{ev})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		ctx, cancel := context.WithCancel(t.Context())
		params := makeRunParams()
		result, err := br.Run(ctx, params)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Cancel caller's context — should not stop the background run
		cancel()

		// Background run should still complete after proceed
		close(ctrl.runProceed)

		select {
		case <-ctrl.runFinished:
			// background run completed even after caller ctx was cancelled
		case <-time.After(2 * time.Second):
			t.Fatal("background run did not finish after caller ctx cancel")
		}
	})

	t.Run("Run records preRunEventCount before starting", func(t *testing.T) {
		histEv := makeEvent(fake.Lorem().Sentence(2))
		dep, ctrl := makeBlockingRunner(t, []*SessionEvent{histEv}, nil)

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		close(ctrl.runProceed)

		// Drain events
		for range result.Events() { //nolint:revive // intentional drain with no action
		}

		select {
		case <-ctrl.runFinished:
		case <-time.After(2 * time.Second):
			t.Fatal("run did not finish")
		}

		// preRunEventCount should be 1 (one history event)
		assert.Equal(t, 1, dep.capturedPreRunEventCount)
	})

	t.Run("after run completes active run is cleaned up", func(t *testing.T) {
		dep, ctrl := makeBlockingRunner(t, nil, nil)

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		close(ctrl.runProceed)

		// Drain events
		for range result.Events() { //nolint:revive // intentional drain with no action
		}

		select {
		case <-ctrl.runFinished:
		case <-time.After(2 * time.Second):
			t.Fatal("run did not finish")
		}

		// Give a tiny moment for the goroutine to clean up
		time.Sleep(10 * time.Millisecond)

		br.mu.Lock()
		_, active := br.activeRuns[params.SessionID]
		br.mu.Unlock()
		assert.False(t, active, "active run should be cleaned up after run completes")
	})

	t.Run("starting a second run on same session while one is active returns error", func(t *testing.T) {
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{makeEvent(fake.Lorem().Sentence(2))})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result1, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result1)

		// Second run with same sessionID while first is still active
		_, err = br.Run(t.Context(), params)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "active")

		// Complete the first run
		close(ctrl.runProceed)
		for range result1.Events() { //nolint:revive // intentional drain with no action
		}
	})

	t.Run("ReadSession with idle session returns all history events IsActive=false", func(t *testing.T) {
		ev1 := makeEvent(fake.Lorem().Sentence(2))
		ev2 := makeEvent(fake.Lorem().Sentence(2))
		dep, _ := makeBlockingRunner(t, []*SessionEvent{ev1, ev2}, nil)

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.NoError(t, err)
		require.NotNil(t, output)
		assert.Equal(t, params.SessionID, output.SessionID())
		assert.False(t, output.IsActive())

		var got []*SessionEvent
		for ev, err := range output.Events() {
			require.NoError(t, err)
			got = append(got, ev)
		}
		assert.Equal(t, []*SessionEvent{ev1, ev2}, got)
	})

	t.Run("ReadSession active session returns pre-run history then live events IsActive=true", func(t *testing.T) {
		preRunEv := makeEvent(fake.Lorem().Sentence(2))
		currentRunEv := makeEvent(fake.Lorem().Sentence(2))
		// Underlying run yields only this turn's events (like ADK); history comes from ReadSession slice.
		dep, ctrl := makeBlockingRunner(t, []*SessionEvent{preRunEv}, []*SessionEvent{currentRunEv})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		runResult, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, runResult)

		// Session is now active, ReadSession should see it
		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.NoError(t, err)
		require.NotNil(t, output)
		assert.True(t, output.IsActive())
		assert.Equal(t, params.SessionID, output.SessionID())

		// Proceed the run so current events are published
		close(ctrl.runProceed)

		// Drain the run result (keep run channel open through subscriber)
		go func() {
			for range runResult.Events() { //nolint:revive // intentional drain with no action
			}
		}()

		var got []*SessionEvent
		for ev, err := range output.Events() {
			require.NoError(t, err)
			got = append(got, ev)
		}

		require.Len(t, got, 2)
		assert.Equal(t, preRunEv, got[0])
		assert.Equal(t, currentRunEv, got[1])
	})

	t.Run("ReadSession deduplication: current-run events capped at preRunEventCount", func(t *testing.T) {
		preRunEv := makeEvent("pre-run")
		currentRunEv := makeEvent("current-run")

		// Mock returns 2 events from ReadSession (pre-run + 1 already persisted current-run event),
		// but preRunEventCount was 1, so only the first event should appear in ReadSession output.
		// Underlying Run yields only the new current-run event (storage holds both).
		dep, ctrl := makeBlockingRunner(
			t,
			[]*SessionEvent{preRunEv, currentRunEv},
			[]*SessionEvent{currentRunEv},
		)

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		dep.preRunHistoryOverride = []*SessionEvent{preRunEv} // only 1 event when called at run start
		runResult, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		// ReadSession returns 2 events (session service has accumulated one current-run event)
		dep.readSessionOverride = []*SessionEvent{preRunEv, currentRunEv}

		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.NoError(t, err)
		assert.True(t, output.IsActive())

		close(ctrl.runProceed)
		go func() {
			for range runResult.Events() { //nolint:revive // intentional drain with no action
			}
		}()

		var got []*SessionEvent
		for ev, err := range output.Events() {
			require.NoError(t, err)
			got = append(got, ev)
		}

		// Should only see preRunEv (from session service capped at 1) then currentRunEv (from EventBus)
		require.Len(t, got, 2)
		assert.Equal(t, preRunEv, got[0])
		assert.Equal(t, currentRunEv, got[1])
	})

	t.Run("underlying Run error closes EventBus and cleans up active run", func(t *testing.T) {
		runErr := errors.New("underlying run failed")
		dep := &errRunDep{runErr: runErr}
		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		var gotErr error
		for _, iterErr := range result.Events() {
			if iterErr != nil {
				gotErr = iterErr
				break
			}
		}
		require.ErrorIs(t, gotErr, runErr)

		// Active run should be cleaned up
		time.Sleep(10 * time.Millisecond)
		br.mu.Lock()
		_, active := br.activeRuns[params.SessionID]
		br.mu.Unlock()
		assert.False(t, active)
	})

	t.Run("underlying Run yields stream error published to bus and cleans up", func(t *testing.T) {
		streamErr := errors.New("stream error")
		dep, ctrl := makeBlockingRunner(t, nil, nil)
		dep.streamErr = streamErr

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		close(ctrl.runProceed)

		var gotErr error
		for _, iterErr := range result.Events() {
			if iterErr != nil {
				gotErr = iterErr
				break
			}
		}
		require.Error(t, gotErr)
		assert.ErrorIs(t, gotErr, streamErr)
	})

	t.Run("ReadSession idle session ReadSession error returns error", func(t *testing.T) {
		readErr := errors.New("session not found")
		dep := &errReadSessionDep{readErr: readErr}
		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.ErrorIs(t, err, readErr)
		assert.Nil(t, output)
	})

	t.Run("ReadSession active session ReadSession error returns error", func(t *testing.T) {
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{makeEvent(fake.Lorem().Sentence(2))})
		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		// Override ReadSession to return error for subsequent calls
		readErr := errors.New("read error")
		dep.readErrOverride = readErr

		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.ErrorIs(t, err, readErr)
		assert.Nil(t, output)

		close(ctrl.runProceed)
		for range result.Events() { //nolint:revive // intentional drain with no action
		}
	})

	t.Run("busSubscriberIterator yield returns false stops iteration", func(t *testing.T) {
		ev1 := makeEvent(fake.Lorem().Sentence(2))
		ev2 := makeEvent(fake.Lorem().Sentence(2))
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{ev1, ev2})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		close(ctrl.runProceed)

		var got []*SessionEvent
		for ev, err := range result.Events() {
			require.NoError(t, err)
			got = append(got, ev)
			break // stop after first event
		}
		assert.Len(t, got, 1)
	})

	t.Run("Shutdown cancels active runs", func(t *testing.T) {
		dep, ctrl := makeBlockingRunner(t, nil, nil)

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Shutdown should cancel active runs
		br.Shutdown()

		// After shutdown the run should be unblocked (cancelled) without us closing runProceed
		select {
		case <-ctrl.runFinished:
		case <-time.After(2 * time.Second):
			// proceed anyway so dep doesn't hang
			close(ctrl.runProceed)
			t.Fatal("background run did not stop after Shutdown")
		}
		// Keep proc from hanging if shutdown worked
		select {
		case <-ctrl.runProceed:
		default:
			close(ctrl.runProceed)
		}
	})
}

func TestBackgroundRunnerPreRunHistoryEdges(t *testing.T) {
	fake := faker.New()

	makeEvent := func(text string) *SessionEvent {
		return &SessionEvent{
			Author:  fake.Internet().User(),
			Content: &SessionEventContent{Parts: []SessionEventPart{{Text: text}}},
		}
	}

	makeRunParams := func() RunParams {
		return RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		}
	}

	makeReadSessionParams := func(sessionID, userID string) ReadSessionParams {
		return ReadSessionParams{
			SessionID: sessionID,
			UserID:    userID,
		}
	}

	makeBlockingRunner := func(
		t *testing.T,
		history []*SessionEvent,
		runEvents []*SessionEvent,
	) (*mockBackgroundRunnerDep, mockRunnerControl) {
		t.Helper()
		ctrl := mockRunnerControl{
			runStarted:  make(chan struct{}),
			runProceed:  make(chan struct{}),
			runFinished: make(chan struct{}),
		}
		dep := newMockBackgroundRunnerDep(t, history, runEvents, ctrl)
		return dep, ctrl
	}

	t.Run("Run tolerates history iterator error when counting pre-run events", func(t *testing.T) {
		liveEv := makeEvent(fake.Lorem().Sentence(2))
		dep := &readHistIterErrDep{ev: liveEv}

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		runResult, err := br.Run(t.Context(), params)
		require.NoError(t, err)
		require.NotNil(t, runResult)

		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.NoError(t, err)
		require.True(t, output.IsActive())

		var got []*SessionEvent
		for ev, evErr := range output.Events() {
			require.NoError(t, evErr)
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		assert.Equal(t, liveEv, got[0])

		for range runResult.Events() { //nolint:revive // drain background run
		}
	})

	t.Run("ReadSession active session with no pre-run history yields only live events", func(t *testing.T) {
		currentRunEv := makeEvent(fake.Lorem().Sentence(2))
		dep, ctrl := makeBlockingRunner(t, nil, []*SessionEvent{currentRunEv})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		runResult, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		output, err := br.ReadSession(t.Context(), makeReadSessionParams(params.SessionID, params.UserID))
		require.NoError(t, err)
		assert.True(t, output.IsActive())

		close(ctrl.runProceed)
		go func() {
			for range runResult.Events() { //nolint:revive // intentional drain
			}
		}()

		var got []*SessionEvent
		for ev, evErr := range output.Events() {
			require.NoError(t, evErr)
			got = append(got, ev)
		}
		require.Len(t, got, 1)
		assert.Equal(t, currentRunEv, got[0])
	})
}

func TestBackgroundRunnerResumeSkipLeading(t *testing.T) {
	fake := faker.New()

	makeEvent := func(text string) *SessionEvent {
		return &SessionEvent{
			Author:  fake.Internet().User(),
			Content: &SessionEventContent{Parts: []SessionEventPart{{Text: text}}},
		}
	}

	makeRunParams := func() RunParams {
		return RunParams{
			UserID:    fake.UUID().V4(),
			SessionID: fake.UUID().V4(),
			Message:   &MessageContent{Parts: []MessagePart{{Text: fake.Lorem().Sentence(3)}}},
		}
	}

	makeBlockingRunner := func(
		t *testing.T,
		history []*SessionEvent,
		runEvents []*SessionEvent,
	) (*mockBackgroundRunnerDep, mockRunnerControl) {
		t.Helper()
		ctrl := mockRunnerControl{
			runStarted:  make(chan struct{}),
			runProceed:  make(chan struct{}),
			runFinished: make(chan struct{}),
		}
		dep := newMockBackgroundRunnerDep(t, history, runEvents, ctrl)
		return dep, ctrl
	}

	t.Run("Run yields underlying stream as-is when session has history", func(t *testing.T) {
		h1 := makeEvent(fake.Lorem().Sentence(2))
		h2 := makeEvent(fake.Lorem().Sentence(2))
		newEv := makeEvent(fake.Lorem().Sentence(2))
		// ADK does not replay session history on Run's iterator — only this invocation's events.
		dep, ctrl := makeBlockingRunner(t, []*SessionEvent{h1, h2}, []*SessionEvent{newEv})

		br := NewBackgroundRunner(BackgroundRunnerDeps{
			Runner: dep,
			Logger: RootTestLogger(),
		})

		params := makeRunParams()
		result, err := br.Run(t.Context(), params)
		require.NoError(t, err)

		close(ctrl.runProceed)

		var got []*SessionEvent
		for ev, err := range result.Events() {
			require.NoError(t, err)
			got = append(got, ev)
		}

		assert.Equal(t, []*SessionEvent{newEv}, got)

		select {
		case <-ctrl.runFinished:
		case <-time.After(2 * time.Second):
			t.Fatal("background run did not finish")
		}
	})
}

type mockRunnerControl struct {
	runStarted  chan struct{}
	runProceed  chan struct{}
	runFinished chan struct{}
}

// listSessionsRecordingDep implements [backgroundRunnerDep] for ListSessions delegation tests.
type listSessionsRecordingDep struct {
	called    bool
	gotParams ListSessionMetadataParams
	result    *ListSessionMetadataResult
}

func (d *listSessionsRecordingDep) Run(_ context.Context, _ RunParams) (*RunResult, error) {
	panic("Run not used")
}

func (d *listSessionsRecordingDep) ReadSession(
	_ context.Context,
	params ReadSessionParams,
) (*ReadSessionResult, error) {
	return NewReadSessionResult(params.SessionID, false, sliceToIter(nil)), nil
}

func (d *listSessionsRecordingDep) ListSessions(
	_ context.Context,
	params ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	d.called = true
	d.gotParams = params
	return d.result, nil
}

// mockBackgroundRunnerDep is a test double for the BackgroundRunner's underlying runner.
// It tracks calls and provides configurable behavior.
type mockBackgroundRunnerDep struct {
	t    *testing.T
	mu   sync.Mutex
	ctrl mockRunnerControl

	// history returned by ReadSession at run-start (preRunEventCount recording)
	preRunHistory []*SessionEvent
	// events yielded by Run
	runEvents []*SessionEvent
	// streamErr is yielded by Run's event iterator after all events
	streamErr error
	// capturedPreRunEventCount is set when Run is called to allow verification
	capturedPreRunEventCount int

	// overrides for subsequent calls (dedup test)
	preRunHistoryOverride []*SessionEvent // if non-nil, used for first Run's ReadSession
	readSessionOverride   []*SessionEvent // if non-nil, used for ReadSession calls after run started
	readErrOverride       error           // if non-nil, ReadSession returns this error
	runCallCount          int
}

// readHistIterErrDep yields an error on the first history event (for pre-run counting), but Run still succeeds.
type readHistIterErrDep struct {
	ev *SessionEvent
}

func (d *readHistIterErrDep) ReadSession(_ context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	histErr := errors.New("history iteration")
	seq := func(yield func(*SessionEvent, error) bool) {
		yield(nil, histErr)
	}
	return NewReadSessionResult(params.SessionID, false, seq), nil
}

func (d *readHistIterErrDep) Run(_ context.Context, params RunParams) (*RunResult, error) {
	seq := func(yield func(*SessionEvent, error) bool) {
		_ = yield(d.ev, nil)
	}
	return NewRunResult(seq, params.SessionID), nil
}

func (d *readHistIterErrDep) ListSessions(
	_ context.Context,
	_ ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return &ListSessionMetadataResult{}, nil
}

// errRunDep is a dep whose Run always returns an error.
type errRunDep struct {
	runErr error
}

func (d *errRunDep) Run(_ context.Context, _ RunParams) (*RunResult, error) {
	return nil, d.runErr
}

func (d *errRunDep) ReadSession(_ context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	return NewReadSessionResult(params.SessionID, false, sliceToIter(nil)), nil
}

func (d *errRunDep) ListSessions(
	_ context.Context,
	_ ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return &ListSessionMetadataResult{}, nil
}

// errReadSessionDep is a dep whose ReadSession always returns an error.
type errReadSessionDep struct {
	readErr error
}

func (d *errReadSessionDep) Run(_ context.Context, params RunParams) (*RunResult, error) {
	return NewRunResult(func(_ func(*SessionEvent, error) bool) {}, params.SessionID), nil
}

func (d *errReadSessionDep) ReadSession(_ context.Context, _ ReadSessionParams) (*ReadSessionResult, error) {
	return nil, d.readErr
}

func (d *errReadSessionDep) ListSessions(
	_ context.Context,
	_ ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return &ListSessionMetadataResult{}, nil
}

func newMockBackgroundRunnerDep(
	t *testing.T,
	history []*SessionEvent,
	runEvents []*SessionEvent,
	ctrl mockRunnerControl,
) *mockBackgroundRunnerDep {
	t.Helper()
	return &mockBackgroundRunnerDep{
		t:             t,
		ctrl:          ctrl,
		preRunHistory: history,
		runEvents:     runEvents,
	}
}

func (m *mockBackgroundRunnerDep) Run(ctx context.Context, params RunParams) (*RunResult, error) {
	m.mu.Lock()
	m.runCallCount++
	events := m.runEvents
	streamErr := m.streamErr
	m.mu.Unlock()

	seq := func(yield func(*SessionEvent, error) bool) {
		// Signal that the run has started.
		select {
		case m.ctrl.runStarted <- struct{}{}:
		default:
		}

		// Block until proceed or ctx cancelled.
		select {
		case <-m.ctrl.runProceed:
		case <-ctx.Done():
			close(m.ctrl.runFinished)
			return
		}

		for _, ev := range events {
			if !yield(ev, nil) {
				close(m.ctrl.runFinished)
				return
			}
		}
		if streamErr != nil {
			yield(nil, streamErr)
		}
		close(m.ctrl.runFinished)
	}

	return NewRunResult(seq, params.SessionID), nil
}

func (m *mockBackgroundRunnerDep) ReadSession(_ context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readErrOverride != nil {
		return nil, m.readErrOverride
	}

	// If readSessionOverride is set (for dedup test), use it.
	if m.readSessionOverride != nil {
		events := m.readSessionOverride
		return NewReadSessionResult(params.SessionID, false, sliceToIter(events)), nil
	}

	// If preRunHistoryOverride is set, use it for the preRunEventCount read.
	if m.preRunHistoryOverride != nil {
		history := m.preRunHistoryOverride
		m.capturedPreRunEventCount = len(history)
		return NewReadSessionResult(params.SessionID, false, sliceToIter(history)), nil
	}

	m.capturedPreRunEventCount = len(m.preRunHistory)
	return NewReadSessionResult(params.SessionID, false, sliceToIter(m.preRunHistory)), nil
}

func (m *mockBackgroundRunnerDep) ListSessions(
	_ context.Context,
	_ ListSessionMetadataParams,
) (*ListSessionMetadataResult, error) {
	return &ListSessionMetadataResult{}, nil
}
