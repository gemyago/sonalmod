# Implementation Summary: Agent API `ServerInterface` — `StartAgentRun` (minimal foundation)

**Plan:** [plan-agent-api-start-handler.md](./plan-agent-api-start-handler.md)

## Overview

The runtime agent HTTP API now implements `ServerInterface` with real `StartAgentRun` and `ContinueAgentRun` paths: JSON decode and validation, injectable session id generation (start only) and mappers, `AgentRunner` execution, and SSE streaming via a dedicated driver. `ContinueAgentRun` uses the path `sessionId` as `RunParams.SessionID` (no new id). Supporting pieces include `RunResult.Events()` for iterators, request and stream-event mappers, and tests from unit level through handler integration and an optional real-runner smoke test.

## Tasks

### Task 1.1: Expose event stream from `RunResult`

Added `(*RunResult) Events() iter.Seq2[*session.Event, error]` delegating to the existing `events` field, with tests for multi-event streams, errors, and empty streams.

### Task 1.2: Injectable ID generator (pattern from `ident`)

Introduced a `Generator` interface for UUID v7 IDs (`NewV7` / `MustNewV7`), `DefaultGenerator` backed by `github.com/google/uuid`, and `MockGenerator` plus helpers aligned with the `ident` pattern, with unit tests for default and mock behavior.

### Task 1.3: Request mapper component

Added `AgentAPIRequestMapper` mapping `UserContent` to `*genai.Content` with validation (non-empty parts, trimmed text, default role `user`) and exported `ErrInvalidUserContent` for 400 responses; table-driven tests cover invalid inputs and happy paths including multiple parts.

### Task 1.4: Stream-event mapper component

Introduced `AgentAPIStreamEventMapper` with `ToStreamEvent` mapping ADK `session.Event` values into `StreamEvent`, using error fields when present and otherwise mapping model metadata and `genai.Content` to `AgentStreamEvent`, with table-driven tests for nil, partial chunks, turn completion, and error-like ADK fields.

### Task 1.5: Injectable SSE response driver

Implemented `StreamEventMapper` and `AgentAPISSEWriter` with `StreamAgentRun` to stream `RunResult` over HTTP as SSE (headers, `sessionBound`, mapped events, flush, `done`, error payloads on mapper/iterator failure), with tests using a stub mapper and fake iterator.

### Task 1.6: Mock runner + `StartAgentRun` handler skeleton

Added `AgentRunStarter` (compile-checked against `*internal.AgentRunner`), `AgentAPIServer` implementing `StartAgentRun` and `ContinueAgentRun` with RFC 9457-style `ProblemDetails` on errors, Mockery for `AgentRunStarter`, and `HandlerFromMux` integration tests covering SSE, validation, optional `model` handling, and continue-run parity with start. Optional `model` in the request body is ignored for v1 (per plan §4.7).

### Task 1.7: Wire real `*AgentRunner` (smoke)

Added a `!release` integration test that builds a real `*AgentRunner` (in-memory session, fake LLM, same pattern as `agentrun_test`), wires `NewAgentAPIServer`, and asserts SSE `sessionBound`, streamed `agent` content including `smoke-ok`, and `done`; the test file comments include a one-line manual command for verbose runs.

### Task 1.8: Documentation

Reviewed `runtime/AGENTS.md` against Task 1.8 (commands, `go generate`, wiring such as `internal/agentapi/server.go`). No edits were required; the API Layer section already described `ServerInterface` / `AgentAPIServer`, dependencies, and regeneration steps.

## Deviations & notes

| Area | Note |
|------|------|
| UUID library | `DefaultGenerator` uses `github.com/google/uuid` (already a runtime dependency) instead of `gofrs/uuid`; API shape still matches `ident`. |
| `AgentAPIRequestMapper` naming | `//nolint:revive` on the struct: planned name stutters with package `agentapi`. |
| Task 1.8 | Review-only; no `runtime/AGENTS.md` changes. |

## Completion

- Lint: ✓
- Tests: ✓
