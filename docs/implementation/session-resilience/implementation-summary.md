# Implementation Summary: Session Resilience — Background Agent Runs & Session Replay

**Plan:** [plan-session-resilience.md](./plan-session-resilience.md)

## Overview

The runtime now decouples agent runs from the HTTP request via `BackgroundRunner` and a replay-capable `EventBus`, exposes `ReadSession` on the runner stack, and serves `GET /sessions/{sessionId}` as SSE with `sessionBound`, `sessionStatus`, replayed `agent` events, and `done`. The Sonal UI calls `readSession` on `/chat/{sessionId}` mount to rebuild history and tail live output when a run is still active.

## Tasks

### Task 1.1: Add `ReadSession` to Runner and fix duplicate session service

Added `ReadSession` on `AgentRunnerFactory` and public `Runner` (`ReadSessionParams` / `ReadSessionResult`), loading sessions via the session service and returning mapped events; fixed duplicate in-memory session wiring by using the shared `rOpts.sessionService` in `AgentRunnerFactoryDeps`.

### Task 2.1: Implement EventBus (replay-capable)

Implemented `EventBus` in `runtime/internal` (publish, subscribe/unsubscribe, `ReplayAndSubscribe`, close/done, replay buffer) with tests for planned behaviors.

### Task 2.2: Implement BackgroundRunner

`BackgroundRunner` wraps the underlying runner with active-run tracking, background execution, EventBus fan-out, unified streams for `Run`/`ReadSession`, and duplicate-run rejection; tests cover idle/active paths, cleanup, and shutdown.

### Task 3.1: Update OpenAPI spec and regenerate

OpenAPI gained `GET /sessions/{sessionId}` (`readSession`), `SessionStatusEvent`, and regenerated TS client pieces; interim stubs were replaced in later tasks.

### Task 3.2: Extend `AgentRunner` interface with `ReadSession`

Extended `agentapi.AgentRunner` with `ReadSession`, updated mocks/tests, and implemented delegation on `*rt.AgentRunner` where needed for `BackgroundRunner` wiring.

### Task 3.3: Implement `ReadSession` handler and SSE methods

Full `ReadSession` HTTP handler, `StreamSessionRead` on the SSE writer (`sessionBound`, `sessionStatus`, mapped events, `done`/`error`), and server/SSE tests.

### Task 3.4: Verify `httpapi.NewHandler` compiles with extended `AgentRunner`

Verification-only: extended `AgentRunner` alias and `AgentRunnerFromRunner` already satisfied compilation and tests.

### Task 4.1: Wire BackgroundRunner in `apps/sonalmod`

HTTP handler receives `BackgroundRunner` via `httpapi.AgentRunnerFromRunner(runner, logger)`; `runtime_test.go` asserts a wired `Runtime` and handler.

### Task 5.1: Update UI types and API client for read-session

`readSession` client, `SessionStatusEvent` / stream guards, and `sessionActive` / `applySessionStatus` in stream state with tests.

### Task 5.2: Implement UI reconnection in Chat.svelte

Mount with `sessionId` runs `loadSession` over SSE, commits idle replay or live turn, and documents reconnect behavior in the wireframe; tests and fixtures added.

## Deviations & notes

- **EventBus (2.1):** Helpers were split to satisfy `gocognit`; behavior matches the plan.
- **BackgroundRunner (2.2):** Unexported `backgroundRunnerDep` avoids clashing with `agentapi.AgentRunner` before interface extension; `ErrSessionBusy` is exported ahead of API use.
- **OpenAPI / codegen (3.1):** Generator path issues (kin-openapi version skew); `api.gen.go` and embedded spec were aligned manually with a small YAML→JSON→gzip→base64 pipeline instead of the normal `go generate` path.
- **AgentRunner ReadSession (3.2):** `ReadSession` was also implemented on `*rt.AgentRunner` for `BackgroundRunner`; params use `rt.BackgroundRunnerReadSessionParams` to match the runner without a duplicate conversion layer.
- **ReadSession handler (3.3):** `NewReadSessionOutput` was added in `background_runner.go` for tests/API construction (mirrors `NewRunResult`).
- **httpapi / sonalmod wiring (3.4 / 4.1):** `AgentRunnerFromRunner` (introduced while fixing a type mismatch between `*agent.Runner` and extended `AgentRunner`) wraps `NewBackgroundRunner` instead of calling `NewBackgroundRunner` directly in `runtime.go`, matching `runtime/AGENTS.md` embedder guidance.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
