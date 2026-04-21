# Plan: Session Resilience — Background Agent Runs & Session Replay

## 1. Introduction / Overview

**Problem:** Today, an agent run is tightly coupled to the HTTP request that started it. When a client (browser tab) disconnects — whether by refresh, network drop, or navigation — the request context is cancelled and the agent run stops immediately. There is no API to read a past or in-flight session, so the UI cannot resume where it left off.

**Goal:** Make agent runs resilient to client disconnections. A run that has started should continue to completion regardless of whether a client is connected. The UI should be able to reconnect to a session (by id), receive the conversation history, and continue streaming live events if the run is still active.

**Explicit non-goal:** Changing the session storage backend. The architecture must work identically with in-memory, file, or database session services — whatever is already configured. No storage wiring, config changes, or service selection belongs in this work.

---

## 2. Business Logic

1. **Background runs:** When the server receives `POST /agent-runs` or `POST /sessions/{sessionId}/agent-runs`, the agent run starts in a background goroutine with a context that is **not** derived from the HTTP request. The SSE response streams events to the client, but if the client disconnects the run keeps going.

2. **Read session:** A new `GET /sessions/{sessionId}` SSE endpoint. It:
   - Reads session events from whatever session service is configured (history).
   - Replays historical events as SSE frames so the client can rebuild the transcript.
   - If the session has an **active** (in-progress) run, the stream stays open and continues forwarding live events until the run completes.
   - If the session is idle (no active run), the stream ends with a `done` event after replaying history.

3. **UI reconnection:** When the user navigates to `/chat/{sessionId}`, the UI calls the read-session endpoint, rebuilds the transcript from replayed events, and if the run is still active, continues showing streaming output.

---

## 3. High Level Architecture

```
┌──────────────┐
│   Browser     │──GET /sessions/{id}──────▶ Read session (SSE replay + live tail)
│  (sonal-ui)  │──POST /agent-runs─────────▶ Start run (SSE, decoupled ctx)
│              │──POST /sessions/{id}/     ▶ Continue run (SSE, decoupled ctx)
│              │   agent-runs
└──────────────┘
        ▲ SSE
        │
┌───────┴──────────────────────────────────────────────┐
│                  Agent API Server                      │
│  (runtime/internal/agentapi)                          │
│                                                        │
│  StartAgentRun ───▶ runner.Run() ──▶ *RunResult        │
│  ContinueAgentRun ▶ runner.Run() ──▶ *RunResult        │
│  ReadSession ─────▶ runner.ReadSession()              │
│  (runner = AgentRunner interface; impl is transparent) │
└───────┬──────────────────────────────────────────────┘
        │
┌───────▼──────────────────────────────────────────────┐
│               BackgroundRunner (new)                   │
│  (runtime/internal)                                   │
│                                                        │
│  - Wraps Runner, delegates Run() for actual execution │
│  - Starts runs in background goroutines               │
│  - Publishes events to a per-session EventBus         │
│  - Tracks active sessions (sessionID → active run)    │
│  - ReadSession: history from Runner + live from bus   │
└───────┬──────────────────────────────────────────────┘
        │
┌───────▼──────────────────────────────────────────────┐
│            Runner (existing, extended)                  │
│  (runtime/internal + runtime/agent)                   │
│                                                        │
│  - Run(): execute agent, return event iterator        │
│  - ReadSession() (new): read session events from      │
│    the configured session.Service                     │
└──────────────────────────────────────────────────────┘
```

**Components involved:**

| Component | Module | Role |
|-----------|--------|------|
| `BackgroundRunner` | `runtime/internal` (new) | Wraps Runner; manages background runs, event fan-out, active-run tracking, ReadSession with live tail |
| `EventBus` | `runtime/internal` (new) | Per-run broadcast mechanism for live event subscribers |
| `AgentRunner` (internal) | `runtime/internal` | Extended with `ReadSession` to read session history from session service |
| `agent.Runner` (public) | `runtime/agent` | Extended with `ReadSession` that delegates to internal `AgentRunner` |
| `AgentAPIServer` | `runtime/internal/agentapi` | HTTP handlers — `AgentRunner` interface extended with `ReadSession`, new ReadSession handler. API server is unaware of `BackgroundRunner`; upper-level wiring decides the implementation. |
| `AgentAPISSEWriter` | `runtime/internal/agentapi` | Extended with session-replay SSE writing |
| `httpapi.NewHandler` | `runtime/httpapi` | Unchanged interface (`AgentRunner`); receives `BackgroundRunner` transparently via wiring |
| `NewRuntime` | `apps/sonalmod/internal` | Creates BackgroundRunner (which satisfies `AgentRunner`), passes to handler |
| `openapi.yaml` | `runtime/internal/agentapi` | New `GET /sessions/{sessionId}` operation |
| `client.ts` | `apps/sonal-ui` | New `readSession` API function |
| `Chat.svelte` | `apps/sonal-ui` | Reconnection logic on mount with sessionId |
| `streamState.ts` | `apps/sonal-ui` | Extended to handle history replay vs live events |

---

## 4. Detailed Architecture

### 4.1 Naming Decisions

**Keep `Runner` as-is (no rename):**
- `Runner` is well-established across the codebase: `agent.Runner` (public), `internal.AgentRunner` (internal), `agentapi.AgentRunner` (interface), `httpapi.AgentRunner` (type alias). Renaming any of these would create churn across many files for no functional benefit.
- Its responsibility is clear and unchanged: "execute an agent run against a session and return an event stream." Adding `ReadSession` is a natural extension — it already owns the session service and app name, and reading a session is a complementary operation to running one.
- A qualifier like `SyncRunner` or `DirectRunner` would only make sense in contrast to `BackgroundRunner`, but the base runner is not "synchronous" in any meaningful sense — it returns a lazy event iterator. The runner IS the runner; the background layer is an orchestration wrapper on top.

**`BackgroundRunner` for the new wrapper:**
- Clearly communicates its key differentiator: runs execute in the background, decoupled from the caller's context/lifecycle.
- "Background" is a well-understood Go concept (background goroutines, `context.Background()`). A Go developer reading the name immediately understands the execution model.
- Alternatives considered and rejected:
  - `ManagedRunner` — "managed" is vague; what is being managed? Doesn't hint at the execution model.
  - `AsyncRunner` — misleading; Go uses goroutines, not async/await. Could imply `Future`-style semantics.
  - `ResidentRunner` — unusual, unclear meaning.
  - `RunOrchestrator` / `RunManager` — loses the "runner" concept the user wants to preserve; also sounds like a generic manager class.
  - `SessionRunner` — "session" is secondary to the background execution aspect; could be confused with the session service.

### 4.2 Extending Runner with ReadSession

**Internal layer (`runtime/internal/agentrun.go`):**

Add `ReadSession` to `AgentRunnerFactory`. It already holds `sessionService`, which is everything needed to read session history. `ReadSession` only needs the session service and app name — it does NOT need the LLM agent or tools, so it lives on the factory (not on `AgentRunner`, which is per-run and requires LLM setup).

```go
type ReadSessionParams struct {
    AppName   string
    SessionID string
    UserID    string
}

type ReadSessionResult struct {
    SessionID string
    Events    []*SessionEvent
}

func (f *AgentRunnerFactory) ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
    resp, err := f.sessionService.Get(ctx, &session.GetRequest{
        AppName:   params.AppName,
        UserID:    params.UserID,
        SessionID: params.SessionID,
    })
    if err != nil {
        return nil, err
    }
    var events []*SessionEvent
    for ev := range resp.Session.Events().All() {
        events = append(events, MapADKSessionEvent(ev))
    }
    return &ReadSessionResult{SessionID: params.SessionID, Events: events}, nil
}
```

**Public layer (`runtime/agent/runner.go`):**

Add `ReadSession` to `agent.Runner`. It delegates to the factory method:

```go
type ReadSessionParams = internal.ReadSessionParams

func (r *Runner) ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error) {
    return r.runnerFactory.ReadSession(ctx, internal.ReadSessionParams{
        AppName:   defaultRunnerAppName,
        SessionID: params.SessionID,
        UserID:    params.UserID,
    })
}
```

### 4.3 EventBus (`runtime/internal/event_bus.go` — new)

A replay-capable broadcast mechanism for a single run's events. Every event published is stored in an internal buffer AND forwarded to live subscriber channels. This enables late subscribers (e.g. ReadSession during an active run) to replay all events from the start of the run without touching the session service.

```go
type EventBus struct {
    mu          sync.Mutex
    buffer      []*SessionEvent         // all events published since run start
    subscribers map[int]chan *SessionEvent
    nextID      int
    closed      bool
    done        chan struct{}
    finalErr    error                   // non-nil if the run ended with an error
}
```

**Methods:**

- `NewEventBus() *EventBus`
- `Publish(event *SessionEvent)` — appends to buffer AND sends to all subscriber channels (under lock).
- `Close(err error)` — marks bus as done, records the final error (if any), closes all subscriber channels, signals `done`.
- `Done() <-chan struct{}` — closed when the bus is closed.

**Subscription methods:**

- `Subscribe() (id int, ch <-chan *SessionEvent)` — returns a channel for live events from this point forward. Used by the initial caller (StartRun) who subscribes before any events are produced.
- `Unsubscribe(id int)` — removes a subscriber.
- `ReplayAndSubscribe(ctx context.Context) iter.Seq2[*SessionEvent, error]` — the key method for late subscribers. Under one lock: copies the current buffer + creates a subscriber channel. Returns an iterator that first yields all buffered events, then yields live events from the channel, stopping when the bus closes or ctx is cancelled. Because the buffer copy and channel registration happen atomically (same lock), there is no gap: events published before the lock are in the buffer; events published after are sent to the channel.

**Why replay-capable:** See §4.5 (deduplication strategy). The EventBus is the canonical source for current-run events, avoiding overlap with the session service.

### 4.4 BackgroundRunner (`runtime/internal/background_runner.go` — new)

The BackgroundRunner wraps the existing `Runner` (via the `AgentRunner` interface used by agentapi) and adds background lifecycle management.

**Responsibilities:**
- Start a run in a background goroutine with a server-scoped context (not the HTTP request context).
- Fan out events: each event produced by the underlying runner is published to a per-run EventBus for any connected SSE subscribers.
- Track which sessions have active (in-progress) runs.
- Provide `ReadSession`: returns a unified event iterator that seamlessly combines pre-run history with current-run events.

**Core types:**

```go
type BackgroundRunner struct {
    runner     AgentRunner         // the existing runner (Run + ReadSession)
    logger     *slog.Logger
    mu         sync.Mutex
    activeRuns map[string]*activeRun // sessionID → activeRun
}

type activeRun struct {
    cancel           context.CancelFunc
    eventBus         *EventBus
    preRunEventCount int  // number of events in session before this run started
}
```

**Interface exposed to agentapi:**

The `agentapi.AgentRunner` interface currently has only `Run`. We extend it with `ReadSession`:

```go
type ReadSessionParams struct {
    SessionID string
    UserID    string
}

type AgentRunner interface {
    Run(ctx context.Context, params RunParams) (*RunResult, error)
    ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionOutput, error)
}
```

`BackgroundRunner` satisfies this interface. Its `Run` starts the agent in a background goroutine and returns a `*RunResult` whose `Events()` iterator reads from the EventBus. The caller (API server) consumes events identically to today — it never sees the EventBus or knows that the run is decoupled from its context. The only behavioral difference is that cancelling the caller's `ctx` stops the SSE stream but does NOT cancel the underlying run — which is exactly the resilience goal.

**`ReadSessionOutput` — unified Events iterator (like `RunResult`):**

```go
type ReadSessionOutput struct {
    sessionID string
    isActive  bool
    events    iter.Seq2[*SessionEvent, error]
}

func (o *ReadSessionOutput) SessionID() string { return o.sessionID }
func (o *ReadSessionOutput) IsActive() bool    { return o.isActive }
func (o *ReadSessionOutput) Events() iter.Seq2[*SessionEvent, error] { return o.events }
```

The `Events()` iterator yields **all** events in order — pre-run history first, then current-run events (if the session is active). The consumer iterates once and gets everything; it does not need to handle two separate data sources. This mirrors `RunResult.Events()` and keeps the reading side simple.

- **Idle session:** `Events()` yields historical events from the session service, then stops. Iterator ends normally.
- **Active session:** `Events()` yields pre-run historical events, then seamlessly continues with current-run events from the EventBus (replay buffer + live), stopping when the run finishes or ctx is cancelled.

**Analysis: Is `IsActive` needed?**

Yes, for one reason: the HTTP handler must write the `sessionStatus` SSE event (`active` / `idle`) **before** iterating the event stream so the UI can show the appropriate indicator upfront (streaming state vs ready composer). The handler cannot infer this from the iterator — it would have to start iterating and wait to see if events stop or keep coming, by which point it has already missed the opportunity to send `sessionStatus` first.

Alternatives considered:
- **Infer from stream behavior**: the UI would not know if the session is active until the stream ends (or doesn't). This degrades UX — the user sees history appearing but has no indication whether to expect more output or start typing.
- **Embed status as a synthetic first event in the iterator**: mixes concerns — session status is API-level metadata, not a session event. Would require the `SessionEvent` type to carry a discriminator it doesn't naturally have.
- **Query active status separately**: possible (BackgroundRunner could expose `IsActive(sessionID) bool`), but then the handler makes two calls instead of one, creating a TOCTOU race.

Conclusion: `IsActive` is a minimal, justified metadata field on `ReadSessionOutput`. It's set once when the output is constructed and is consistent with the iterator's behavior.

**Run flow (BackgroundRunner.Run):**
1. `BackgroundRunner.Run(ctx, params)` — before starting the run, reads the current event count from the session service via `runner.ReadSession(...)` and stores it as `preRunEventCount` on the active run entry. Creates a server-scoped context, calls the underlying `runner.Run(bgCtx, params)` to get a `*RunResult`, spawns a goroutine that iterates the underlying `RunResult.Events()` and publishes each event to a per-run EventBus.
2. Returns a new `*RunResult` whose `Events()` iterator reads from the EventBus (via `Subscribe()` — the initial caller subscribes before any events are produced, so no replay is needed). The caller's `ctx` controls iteration (stops on cancel), but the background goroutine keeps running.
3. The HTTP handler calls `sse.StreamAgentRun(ctx, w, result)` exactly as before — it doesn't know the events come from an EventBus.
4. When the underlying iterator finishes, the goroutine closes the EventBus and removes the active run entry.

**ReadSession flow (BackgroundRunner.ReadSession):**
1. `BackgroundRunner.ReadSession(ctx, params)` — checks `activeRuns[params.SessionID]`.
2. **If idle** (no active run): delegates to `runner.ReadSession(ctx, params)` for all historical events. Wraps them in an iterator. Returns `ReadSessionOutput{isActive: false, events: historyIter}`.
3. **If active**: reads full event list from `runner.ReadSession(...)`, takes only the first `preRunEventCount` events (pre-run history). Gets the current-run events from `eventBus.ReplayAndSubscribe(ctx)`. Chains the two into a single iterator: pre-run history → current-run (replay + live). Returns `ReadSessionOutput{isActive: true, events: chainedIter}`.

### 4.5 Event Deduplication Strategy

During an active run, both the session service and the EventBus accumulate events from the same run. The ADK runner calls `session.Service.AppendEvent()` for each non-partial event as it's produced, so the session service continuously grows. Meanwhile, the EventBus receives every event (including partials) from the background goroutine. If ReadSession naively combined "all events from session service" + "all events from EventBus", it would produce duplicates for every non-partial event from the current run.

**The solution: clean partitioning by event count.**

When `BackgroundRunner.StartRun()` begins, it records `preRunEventCount` — the number of events already stored in the session. This creates a clean, immutable boundary:

- **Pre-run events** (indices `0..preRunEventCount-1`): come exclusively from the session service. These are events from previous runs/turns. They do not change during the current run.
- **Current-run events** (from the run start onward): come exclusively from the EventBus. The bus stores all events it has published (including partials) in its replay buffer.

When `ReadSession` is called during an active run:
1. Read all events from the session service (which includes pre-run + some current-run events already persisted).
2. **Take only the first `preRunEventCount` events** — this gives us pre-run history cleanly, ignoring any current-run events that the session service has accumulated.
3. Get current-run events from `eventBus.ReplayAndSubscribe(ctx)` — this replays all events from the run start (buffered) and then continues with live events.
4. Chain (2) and (3) into a single iterator. **Zero duplicates** — pre-run comes from session service (capped), current-run comes from EventBus (complete).

**Why this works without ID-based dedup:**
- `preRunEventCount` is captured once, before the run starts, and never changes.
- Pre-run events in the session service are immutable (no new events are inserted before the boundary).
- The EventBus is the sole canonical source for current-run events. It has every event the run has produced (stored in its replay buffer), including partials that the session service skips.
- No event ID comparison, timestamp filtering, or hashing is needed.

**Important note on partial events:** The session service's `AppendEvent` skips partial events (`if event.Partial { return nil }`), so session history only contains non-partial (final) events. The EventBus stores ALL events including partials, which is desirable for live streaming — the UI gets incremental text. When replaying historical events (pre-run), only non-partial events appear, which is correct because those represent complete turns. When replaying current-run events from the bus, both partials and finals appear, giving the full streaming experience.

### 4.5 OpenAPI Spec Changes (`runtime/internal/agentapi/openapi.yaml`)

Add a new operation:

```yaml
/sessions/{sessionId}:
  get:
    tags: [Agent runs]
    operationId: readSession
    summary: Read session events (replay history and optionally tail live run)
    parameters:
      - $ref: '#/components/parameters/SessionId'
      - name: userId
        in: query
        required: true
        schema:
          type: string
          minLength: 1
    responses:
      '200':
        description: SSE stream replaying session events; stays open if a run is active.
        content:
          text/event-stream:
            schema:
              type: string
      '404':
        description: Session not found.
        content:
          application/problem+json:
            schema:
              $ref: '#/components/schemas/ProblemDetails'
```

Add a new stream event type for session status:

```yaml
SessionStatusEvent:
  type: object
  required: [event, status]
  properties:
    event:
      type: string
      const: sessionStatus
    status:
      type: string
      enum: [active, idle]
      description: Whether a run is currently in progress for this session.
```

Add `sessionStatus` to the `StreamEvent` discriminator mapping.

The stream for `readSession` would be:
1. `sessionBound` event (with sessionId)
2. `sessionStatus` event (`active` or `idle`)
3. Zero or more `agent` events (replayed from history)
4. If status is `active`: continues with live `agent` events until the run finishes
5. `done` event

### 4.6 Agent API Server Changes (`runtime/internal/agentapi/server.go`)

**Updated dependencies:**
- `AgentAPIServer` keeps the same `runner AgentRunner` field. The `AgentRunner` interface is extended with `ReadSession` (see §4.4). The server does not know or care which implementation is behind the interface.
- `StartAgentRun` and `ContinueAgentRun` continue calling `runner.Run(ctx, params)` and streaming `RunResult.Events()` as SSE — no change in their logic. When the wired implementation is `BackgroundRunner`, the run happens in a background goroutine and the returned `RunResult` iterator reads from the EventBus transparently.
- The HTTP request context (`r.Context()`) is still passed to `Run` and used for the SSE write loop. With `BackgroundRunner`, cancelling this ctx stops event delivery to the caller but does NOT cancel the underlying run — that's handled by the implementation, not the API server.

**New handler:** `ReadSession(w http.ResponseWriter, r *http.Request, sessionID SessionId)` — parses `userId` query param, calls `runner.ReadSession(...)`, streams the result as SSE.

### 4.7 SSE Writer Changes (`runtime/internal/agentapi/sse.go`)

Add a method `StreamSessionRead` for streaming a read-session result. It differs from `StreamAgentRun` in that:
- It writes a `sessionStatus` event (active/idle) after `sessionBound`.
- It iterates `ReadSessionOutput.Events()` — a single unified iterator that yields pre-run history then (if active) current-run events seamlessly. The SSE writer does not distinguish between historical and live events; it just maps and writes each one.

`StreamAgentRun` (used by start/continue) stays unchanged — it reads from `RunResult.Events()` as today. The `BackgroundRunner` implementation returns a `RunResult` whose iterator internally reads from the EventBus, but the SSE writer is unaware of this.

### 4.8 httpapi Handler Changes (`runtime/httpapi/handler.go`)

- `HandlerArgs.AgentRunner` type stays `agentapi.AgentRunner` (the same type alias). The interface now includes `ReadSession`, but the `HandlerArgs` type and `NewHandler` signature are unchanged.
- `NewHandler` continues to pass `args.AgentRunner` into `AgentAPIServer` via `ServerParams`. When the caller (app wiring) passes a `BackgroundRunner`, it satisfies `AgentRunner` transparently.
- No type alias changes needed.

### 4.9 App Wiring (`apps/sonalmod/internal/runtime.go`)

- After creating `agent.Runner` (unchanged), wrap it in an `internal.BackgroundRunner`.
- Pass the `BackgroundRunner` to `httpapi.NewHandler` instead of the bare runner.
- No storage changes. The in-memory session service that `agent.NewRunner` already creates is used as-is.

### 4.10 UI: API Client (`apps/sonal-ui/src/lib/agentapi/client.ts`)

New function:

```typescript
export async function readSession(params: ReadSessionParams): Promise<Response> {
  // GET /sessions/{sessionId}?userId=...
}
```

This is a GET request with query parameters. The response is an SSE stream parsed with the existing `parseAgentSseJsonStream`.

### 4.11 UI: Types (`apps/sonal-ui/src/lib/agentapi/types.ts`)

Add `SessionStatusEvent` type and update `StreamEvent` union and `isStreamEvent` guard to include `sessionStatus`.

### 4.12 UI: Stream State (`apps/sonal-ui/src/lib/agentapi/streamState.ts`)

Add handler for `sessionStatus` event. The `AgentRunStreamState` gains a `sessionActive` boolean so the UI knows whether to expect more events or show the composer as ready.

### 4.13 UI: Chat Page (`apps/sonal-ui/src/pages/Chat.svelte`)

**On mount with `params.sessionId`:**
1. Call `readSession({ sessionId, userId })`.
2. Process the SSE stream: `sessionBound` confirms the session, `sessionStatus` tells us if a run is active, `agent` events rebuild the transcript, `done` marks completion.
3. If `sessionStatus` is `active`, show streaming UI ("Thinking..." or partial text).
4. If `sessionStatus` is `idle`, just populate the message history and show the composer.

**On send (existing flow):** Use `continueAgentRun` as today — the BackgroundRunner handles the actual execution.

### 4.14 OpenAPI Codegen

After modifying `openapi.yaml`, regenerate:
- Go: `go generate ./internal/agentapi` (runtime module)
- TS: `make generate-api` (sonal-ui module)

---

## 5. Key Architectural Decisions

1. **Runner + BackgroundRunner layering (not a "RunManager").** The existing Runner concept is preserved and extended with `ReadSession`. A new `BackgroundRunner` wraps it to add background lifecycle management. This keeps the core runner focused on agent execution and session access, while the background layer is strictly about decoupling from the caller context and managing active-run state. The two responsibilities are cleanly separated and independently testable.

2. **Server-scoped context for runs, not request context.** The BackgroundRunner creates a background context for each run (not derived from the HTTP request). The HTTP request context is only used for the SSE write loop to detect client disconnection. This is the core change that enables resilience.

3. **Replay-capable EventBus for fan-out and late-subscriber catch-up.** The EventBus stores all events from a run in a replay buffer AND forwards them to live subscriber channels. This enables late subscribers (ReadSession) to get the complete current-run event stream without touching the session service. The EventBus is discarded when the run finishes, so memory is bounded by run duration. Pre-run history comes from the session service; current-run events come exclusively from the EventBus — no deduplication needed (see §4.5).

4. **Event deduplication via count-based partitioning (not ID matching).** The BackgroundRunner records the number of session events before a run starts (`preRunEventCount`). ReadSession uses this to cleanly separate pre-run history (from session service, capped at that count) from current-run events (from EventBus replay). This avoids the need for event IDs, timestamps, or any overlap detection.

5. **Read session uses the same SSE protocol.** Reusing the existing `StreamEvent` discriminated union (plus a new `sessionStatus` event type) means the UI's SSE parser and reducer need minimal changes.

6. **Storage is orthogonal.** The architecture works with any `session.Service` implementation (in-memory, file, database). BackgroundRunner does not depend on the specific storage backend. With in-memory storage, session history is available as long as the server is running; with persistent storage, it survives restarts. This work does not change which storage is used.

7. **BackgroundRunner is internal and transparent to the API layer.** It sits in `runtime/internal`, implements the existing `agentapi.AgentRunner` interface, and is not part of the public contract. The API server and `httpapi.NewHandler` accept `AgentRunner` — they never reference `BackgroundRunner` directly. Upper-level wiring decides whether to inject a bare runner or a `BackgroundRunner`.

8. **No run ID — session ID is the primary key.** The existing design uses session ID for continuity. Since a session can only have one active run at a time, session ID is sufficient for tracking.

---

## 6. Uncertainties

1. **Concurrent runs on the same session.** What happens if a client sends a new message while a previous run is still active? The BackgroundRunner should probably reject or queue it. Current ADK behavior is unclear — needs investigation. The plan assumes one active run per session (reject if busy).

2. **Server restart during an active run.** If using in-memory sessions, all state is lost on restart. If using persistent sessions, history survives but the active run does not — the read-session endpoint would return `idle` status. The user would need to re-send their message. This is acceptable.

3. **Memory pressure from EventBus replay buffer.** The EventBus stores all events from the current run in memory (replay buffer). For a typical agent run (tens to hundreds of events), this is negligible. For very long runs with thousands of events, the buffer grows proportionally. This is acceptable because (a) the buffer is discarded when the run finishes, (b) each `SessionEvent` is small (text parts + metadata), and (c) the session service stores the same data on disk anyway. Subscriber channels also need reasonable buffer sizes (e.g. 256) to avoid blocking the publisher goroutine when a slow subscriber can't keep up.

4. **userId handling for readSession.** Currently `userId` is sent in the POST body. For GET, it would be a query parameter. This is a slight inconsistency but acceptable for a read-only endpoint. An alternative is using headers or auth tokens — deferred to future auth work.

5. **Graceful shutdown.** When the server shuts down, active runs should be cancelled via context. The BackgroundRunner's per-run contexts should be derived from a server lifecycle context. This needs to be wired through the DI layer.

6. **Fix the duplicate `session.InMemoryService()` in `agent.NewRunner`.** Currently `runner.go` creates `session.InMemoryService()` on line 73 (in `rOpts`) and again on line 111 (in `AgentRunnerFactoryDeps`), producing two separate in-memory stores. The factory build should use `rOpts.sessionService`. This is a pre-existing bug that should be fixed as part of this work, but it is NOT a storage change — it just makes the runner internally consistent.

---

## 7. Related Files

### Files to modify

| File | Module | Change |
|------|--------|--------|
| `runtime/agent/runner.go` | runtime | Add `ReadSession` method; fix duplicate session service bug |
| `runtime/internal/agentrun.go` | runtime | Add `ReadSession` method to `AgentRunner` and/or factory |
| `runtime/internal/agentapi/openapi.yaml` | runtime | Add `GET /sessions/{sessionId}`, `SessionStatusEvent` |
| `runtime/internal/agentapi/server.go` | runtime | Use BackgroundRunner, add `ReadSession` handler |
| `runtime/internal/agentapi/sse.go` | runtime | Add `StreamSessionRead` method |
| `runtime/internal/agentapi/stream_event_mapper.go` | runtime | Map `sessionStatus` event if needed |
| `runtime/httpapi/handler.go` | runtime | No interface change; `AgentRunner` type alias now includes `ReadSession` via upstream change |
| `apps/sonalmod/internal/runtime.go` | apps/sonalmod | Create BackgroundRunner wrapping Runner, pass to handler |
| `apps/sonal-ui/src/lib/agentapi/client.ts` | apps/sonal-ui | Add `readSession` function |
| `apps/sonal-ui/src/lib/agentapi/types.ts` | apps/sonal-ui | Add `SessionStatusEvent`, update `StreamEvent` |
| `apps/sonal-ui/src/lib/agentapi/streamState.ts` | apps/sonal-ui | Handle `sessionStatus` event |
| `apps/sonal-ui/src/pages/Chat.svelte` | apps/sonal-ui | Reconnection logic on mount |
| `apps/sonal-ui/ui-wireframe.md` | apps/sonal-ui | Update with reconnection behavior |

### Files to create

| File | Module | Purpose |
|------|--------|---------|
| `runtime/internal/background_runner.go` | runtime | BackgroundRunner: wraps Runner with background lifecycle |
| `runtime/internal/background_runner_test.go` | runtime | Tests for BackgroundRunner |
| `runtime/internal/event_bus.go` | runtime | EventBus: fan-out mechanism for session events |
| `runtime/internal/event_bus_test.go` | runtime | Tests for EventBus |

### Files regenerated (after spec changes)

| File | Module | How |
|------|--------|-----|
| `runtime/internal/agentapi/api.gen.go` | runtime | `go generate ./internal/agentapi` |
| `apps/sonal-ui/src/lib/agentapi/agentapi.generated.ts` | apps/sonal-ui | `make generate-api` |

---

## 8. Task List

> **Note:** TDD approach must be followed. Module-specific task completion protocol must be followed for each task.

---

**Task 1.1: Add `ReadSession` to Runner and fix duplicate session service**
- In `runtime/internal/agentrun.go`:
  - Add `ReadSessionParams` struct (`AppName`, `SessionID`, `UserID`)
  - Add `ReadSessionResult` struct (`SessionID` + `[]*SessionEvent`)
  - Add `ReadSession(ctx, params ReadSessionParams) (*ReadSessionResult, error)` to `AgentRunnerFactory` — reads session from session service, maps ADK events to `SessionEvent` via `MapADKSessionEvent`
- In `runtime/agent/runner.go`:
  - Export `ReadSessionParams = internal.ReadSessionParams` (type alias, like `RunParams`)
  - Add `ReadSession(ctx, params ReadSessionParams) (*ReadSessionResult, error)` to `Runner` — delegates to factory, filling in `AppName` from `defaultRunnerAppName`
  - Fix line 111: use `rOpts.sessionService` instead of `session.InMemoryService()`
- Write failing tests:
  - `ReadSession` returns mapped events from session service
  - `ReadSession` with unknown session returns error
  - Runner uses the same session service instance throughout (no duplicate)
- Run affected tests: `go test -v ./internal --run TestAgentRunnerFactory` and `go test -v ./agent/... --run TestRunner`
  - Verify failure is expectation-based (not compilation errors)
- Implement the methods and bug fix
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-1.1.md`

---

**Task 2.1: Implement EventBus (replay-capable)**
- Create `runtime/internal/event_bus.go` with `EventBus` struct
- Must store all published events in an internal replay buffer
- Methods:
  - `NewEventBus() *EventBus`
  - `Publish(*SessionEvent)` — appends to buffer + sends to all subscriber channels
  - `Subscribe() (id int, ch <-chan *SessionEvent)` — live-only subscriber (for initial StartRun caller)
  - `Unsubscribe(id int)` — removes subscriber
  - `ReplayAndSubscribe(ctx context.Context) iter.Seq2[*SessionEvent, error]` — atomically copies buffer + registers channel (under one lock); returns iterator that yields buffered events then live events, stopping on bus close or ctx cancellation
  - `Close(err error)` — marks done, records final error, closes all subscriber channels
  - `Done() <-chan struct{}`
- Write failing tests in `runtime/internal/event_bus_test.go`:
  - `Subscribe`: subscriber receives published events
  - `Subscribe`: multiple subscribers each receive all events
  - `Unsubscribe`: stops delivery to that subscriber
  - `Close`: signals done and closes subscriber channels
  - `Publish` after `Close` is a no-op (does not panic)
  - `ReplayAndSubscribe`: late subscriber receives all past events then live events
  - `ReplayAndSubscribe`: context cancellation stops iteration
  - `ReplayAndSubscribe`: bus close after some live events ends iteration (with error if non-nil)
  - `ReplayAndSubscribe`: buffer copy and channel registration are atomic (no gap)
- Run affected tests: `go test -v ./internal --run TestEventBus`
  - Verify failure is expectation-based
- Implement EventBus logic
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-2.1.md`

---

**Task 2.2: Implement BackgroundRunner**
- Create `runtime/internal/background_runner.go` with `BackgroundRunner` struct
- `BackgroundRunner` satisfies the `agentapi.AgentRunner` interface (`Run` + `ReadSession`)
- Depends on: underlying `AgentRunner` (for delegating `Run` and `ReadSession` for history), `EventBus`
- Methods:
  - `NewBackgroundRunner(deps BackgroundRunnerDeps) *BackgroundRunner`
  - `Run(ctx, params RunParams) (*RunResult, error)`:
    - Records `preRunEventCount` by calling `runner.ReadSession(...)` first
    - Starts run in background goroutine via the underlying runner
    - Returns a `*RunResult` whose `Events()` reads from EventBus via `Subscribe()`
    - Caller's `ctx` controls event delivery; background run continues independently
  - `ReadSession(ctx, params ReadSessionParams) (*ReadSessionOutput, error)`:
    - Returns `ReadSessionOutput` with a **single unified `Events()` iterator**
    - **Idle**: iterator yields all events from `runner.ReadSession()`, then stops
    - **Active**: iterator yields first `preRunEventCount` events from `runner.ReadSession()` (pre-run history), then chains with `eventBus.ReplayAndSubscribe(ctx)` (current-run: replay buffer + live). No duplicates — see §4.5.
    - `IsActive()` returns whether a run is in progress (needed for `sessionStatus` SSE event)
  - `Shutdown()` — cancels all active runs
- Define `ReadSessionOutput` struct: `sessionID string`, `isActive bool`, `events iter.Seq2[*SessionEvent, error]` with accessor methods `SessionID()`, `IsActive()`, `Events()`
- Write failing tests in `runtime/internal/background_runner_test.go`:
  - Run returns a RunResult whose Events() yields events from the underlying runner
  - Run executes in background — caller ctx cancellation does not stop the run
  - Run records preRunEventCount before starting
  - After run completes, active run is cleaned up
  - Starting a second run on the same session while one is active returns error
  - ReadSession with idle session returns unified iterator with all history, IsActive=false
  - ReadSession with active session returns unified iterator: pre-run history then current-run events from bus, IsActive=true
  - ReadSession deduplication: current-run events from session service are excluded (capped at preRunEventCount)
  - Shutdown cancels active runs
- Run affected tests: `go test -v ./internal --run TestBackgroundRunner`
  - Verify failure is expectation-based
- Implement BackgroundRunner logic
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-2.2.md`

---

**Task 3.1: Update OpenAPI spec and regenerate**
- Add `GET /sessions/{sessionId}` operation to `runtime/internal/agentapi/openapi.yaml`
  - Query parameter: `userId` (required)
  - Response: SSE stream (same as other endpoints)
  - 404 response for session not found
- Add `SessionStatusEvent` schema to components
- Add `sessionStatus` to `StreamEvent` discriminator mapping
- Regenerate Go code: `go generate ./internal/agentapi` (from `runtime/`)
- Regenerate TS types: `make generate-api` (from `apps/sonal-ui/`)
- Run lint in both modules to ensure generated code is valid
- Run module lint: `make lint` (from `runtime/`) and `make lint` (from `apps/sonal-ui/`)
- Write summary to `doc/implementation/session-resilience/summary-task-3.1.md`

---

**Task 3.2: Extend `AgentRunner` interface with `ReadSession`**
- Add `ReadSessionParams` struct (`SessionID`, `UserID`) and `ReadSessionOutput` type to `agentapi` package (or re-export from `internal`)
- Add `ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionOutput, error)` to the `agentapi.AgentRunner` interface
- Define `ReadSessionOutput` in the `agentapi` package (or re-export from `internal`)
- `AgentAPIServer` keeps using `runner AgentRunner` — no field change, no new interface
- `StartAgentRun` and `ContinueAgentRun` logic is unchanged — they call `runner.Run()` and stream `RunResult.Events()` as before. Background execution is transparent via the wired implementation.
- Update `ServerParams` only if needed (add `ReadSession`-related dependencies)
- Update existing mock/test implementations of `AgentRunner` to satisfy the new method
- Write failing tests:
  - Existing start/continue tests still pass (no behavioral change at API level)
  - Mock `AgentRunner` now includes `ReadSession` stub
- Run affected tests: `go test -v ./internal/agentapi --run TestAgentAPIServer`
  - Verify failure is expectation-based
- Implement the changes
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-3.2.md`

---

**Task 3.3: Implement `ReadSession` handler and SSE methods**
- Add `ReadSession(w, r, sessionID)` to `AgentAPIServer`
- Implementation:
  - Parse `userId` from query params
  - Call `runner.ReadSession(ctx, params)` to get `ReadSessionOutput`
  - Stream the result: `sessionBound`, `sessionStatus` (from `output.IsActive()`), then iterate `output.Events()` writing each as SSE, then `done`
  - The unified iterator handles pre-run vs current-run seamlessly — the handler just maps and writes each event
- Add `StreamSessionRead` method to `AgentAPISSEWriter`
- Write failing tests:
  - ReadSession with idle session replays history and sends done
  - ReadSession with active session replays history then streams live events
  - ReadSession with unknown session returns 404
  - Invalid userId returns 400
- Run affected tests: `go test -v ./internal/agentapi --run TestAgentAPIServer`
  - Verify failure is expectation-based
- Implement the handler and SSE method
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-3.3.md`

---

**Task 3.4: Verify `httpapi.NewHandler` compiles with extended `AgentRunner`**
- No interface change in `httpapi` — `AgentRunner` type alias picks up the new `ReadSession` method automatically
- Verify existing tests compile and pass with the extended interface (test mocks may need the new method stub)
- Run module lint and tests: `make lint && make test` (from `runtime/`)
- Write summary to `doc/implementation/session-resilience/summary-task-3.4.md`

---

**Task 4.1: Wire BackgroundRunner in `apps/sonalmod`**
- Update `apps/sonalmod/internal/runtime.go`:
  - After creating `agent.Runner` (unchanged), wrap it in `internal.NewBackgroundRunner(...)`
  - Pass BackgroundRunner to `httpapi.NewHandler` instead of the bare runner
- No storage changes — the existing in-memory session service stays as-is
- Write failing tests:
  - Runtime creation succeeds with BackgroundRunner
  - BackgroundRunner is passed to httpapi handler
- Run affected tests: `go test -v ./internal --run TestRuntime`
  - Verify failure is expectation-based
- Implement the wiring
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `apps/sonalmod/`)
- Write summary to `doc/implementation/session-resilience/summary-task-4.1.md`

---

**Task 5.1: Update UI types and API client for read-session**
- Update `apps/sonal-ui/src/lib/agentapi/types.ts`:
  - Add `SessionStatusEvent` type
  - Update `StreamEvent` union type
  - Update `isStreamEvent` guard to include `sessionStatus`
- Update `apps/sonal-ui/src/lib/agentapi/client.ts`:
  - Add `readSession(params: ReadSessionParams): Promise<Response>` (GET request with query params)
- Update `apps/sonal-ui/src/lib/agentapi/streamState.ts`:
  - Add `sessionActive` field to `AgentRunStreamState`
  - Add handler for `sessionStatus` event in `applyAgentStreamEvent`
- Write failing tests:
  - `isStreamEvent` recognizes `sessionStatus` events
  - `applyAgentStreamEvent` correctly handles `sessionStatus` event
  - `readSession` client function makes GET request with correct params
- Run affected tests: `npm run test:run` (from `apps/sonal-ui/`)
  - Verify failure is expectation-based
- Implement the changes
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `apps/sonal-ui/`)
- Write summary to `doc/implementation/session-resilience/summary-task-5.1.md`

---

**Task 5.2: Implement UI reconnection in Chat.svelte**
- Update `apps/sonal-ui/src/pages/Chat.svelte`:
  - On mount, if `params.sessionId` is present, call `readSession(...)` instead of just opening the composer
  - Process the SSE stream: rebuild `messages` from replayed `agent` events
  - If `sessionStatus` is `active`, show streaming UI
  - If `sessionStatus` is `idle`, show full history and ready composer
  - When user sends a new message, use `continueAgentRun` as before
- Update `apps/sonal-ui/ui-wireframe.md` with reconnection behavior
- Write failing tests:
  - On mount with sessionId, readSession is called
  - Historical events populate messages array
  - Active session shows streaming state
  - Idle session shows composer ready
- Run affected tests: `npm run test:run` (from `apps/sonal-ui/`)
  - Verify failure is expectation-based
- Implement the reconnection logic
- Run affected tests and verify all pass
- Run module lint and tests: `make lint && make test` (from `apps/sonal-ui/`)
- Write summary to `doc/implementation/session-resilience/summary-task-5.2.md`

---

**Task 6.1: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
