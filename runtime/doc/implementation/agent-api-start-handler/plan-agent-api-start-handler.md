# Plan: Agent API `ServerInterface` — `StartAgentRun` (minimal foundation)

## 1. Introduction / overview

The runtime already generates OpenAPI types and a `net/http` (Go 1.22+) server stub via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) (`std-http-server`), including `ServerInterface` in `runtime/internal/agentapi/api.gen.go`. Nothing yet implements that interface or wires it into a live HTTP server.

**Goal:** Implement a **minimal but real** `StartAgentRun` handler (`POST /v1/agent-runs`): parse JSON, start a new session, stream **Server-Sent Events** (`text/event-stream`) aligned with `openapi.yaml`, and integrate with existing `AgentRunner` / `RunResult` code. **Priority** is project standards: **dependency injection**, **testability** (httptest + mocks), and a clear extension point—not full product completeness (e.g. auth, full event mapping, or `ContinueAgentRun` behavior).

**Non-goals for this phase:** Full `ContinueAgentRun` implementation, request validation middleware, production server binary wiring (unless already trivial), exhaustive SSE edge cases.

**Update (2026-03):** `ContinueAgentRun` (`POST /sessions/{sessionId}/agent-runs`) is implemented in the runtime: same body validation and SSE streaming as `StartAgentRun`, with `sessionId` taken from the path. Sections below that describe continue as 501 reflect the original plan only.

---

## 2. Business logic (words)

- **Start:** Client sends `AgentRunRequest` (`userId`, `message`, optional `model`). Server allocates a **new** `sessionId`, binds the ADK session (via existing `AgentRunner` / session service behavior), runs the agent with streaming, and returns an **SSE** response.
- **Stream contract:** First logical event is **`sessionBound`** with the assigned `sessionId`. Subsequent **`agent`** events carry streamed content; stream ends with **`done`** (and optionally **`error`** on failure), matching `StreamEvent` in the spec.
- **Errors:** Malformed JSON or missing required fields → **400** with problem-details shape where practical. Runner / internal failures during streaming → **`error`** SSE event and/or **500** depending on whether headers were already committed (see decisions below).

---

## 3. High-level architecture

| Piece | Role |
|--------|------|
| **`agentapi.ServerInterface`** (generated) | Contract for HTTP handlers (`StartAgentRun`, `ContinueAgentRun`). |
| **New implementation type** (in [`runtime/internal/agentapi`](../../../internal/agentapi), package `agentapi`) | Implements `ServerInterface`; holds injected dependencies; **no business logic in `api.gen.go`** (hand-written code lives in sibling `.go` files in the same package). |
| **Runner abstraction (interface)** | Consumer-defined small interface (e.g. `Run(ctx, RunParams) (*RunResult, error)`) satisfied by `*AgentRunner`, for tests. |
| **`agentapi.HandlerFromMux` / `HandlerWithOptions`** | Registers routes on a `ServeMux`; tests and future `cmd` use the same wiring as [oapi-codegen std-http example](https://github.com/oapi-codegen/oapi-codegen/tree/main?tab=readme-ov-file#impl-stdhttp). |
| **`RunResult` / event iteration** | Today `RunResult` hides the event iterator behind string consumers; SSE needs **raw `*session.Event` stream** (or an explicit iterator API) to map to `agentapi.StreamEvent`. |
| **ID generator (injectable)** | Same *approach* as [`tools/firecrawl/internal/system/ident`](../../../../tools/firecrawl/internal/system/ident): a **`Generator` interface**, default implementation, and test double so session ids are deterministic in tests—**no** direct `uuid.New…` calls inside the handler. |
| **Three boundary components (injectable)** | **(1) Request / ADK input mapper** (handler), **(2) stream / API output mapper** (injected into the driver), **(3) SSE response driver**—see §4.3–§4.4. Handler decodes, runs, then hands **`RunResult`** to the driver; the driver streams SSE using mappers internally. |

---

## 4. Detailed architecture

### 4.1 New implementation struct (package `agentapi`)

- Add a struct (name TBD, e.g. `AgentAPIServer`) that embeds or holds:
  - **Runner** interface (see below).
  - **Logger** `*slog.Logger` (injected; no global logger), consistent with `AgentRunner` / project guide.
  - **ID generator** — inject an interface modeled after [`ident.Generator`](../../../../tools/firecrawl/internal/system/ident/generator.go): methods that produce a new id (e.g. UUID v7 with `NewV7() (ID, error)` and optionally `MustNewV7()`), abstract type for the id (`string` or a type alias), and a **`DefaultGenerator`** wired in production. Tests inject a mock or stub that returns fixed ids (same spirit as [`ident.MockGenerator`](../../../../tools/firecrawl/internal/system/ident/mock.go)). The runtime module may use `google/uuid` or another library under the default impl; the important part is **injectability and test control**, not sharing the firecrawl package verbatim (different module boundaries).
  - **Request mapper** (component 1) — see §4.4.
  - **Stream-event mapper** (component 2) — see §4.4; **injected into the SSE driver**, not held directly by the handler.
  - **SSE response driver** (component 3) — see §4.3; takes **`RunResult`** (and `http.ResponseWriter` / context); owns iteration, mapping, and framing.
- Implement `StartAgentRun(w, r)`:
  - Use `r.Context()` for cancellation.
  - **Decode** body: `json.NewDecoder(r.Body).Decode(&req)` into `agentapi.AgentRunRequest` (or `StartAgentRunJSONRequestBody` alias). Limit body size if the project has a standard max-bytes pattern; otherwise document a follow-up.
  - **Validate** minimal fields: non-empty `userId`, non-nil / usable `message` (align with how `genai.Content` is built—empty parts may be invalid).
  - **Session id:** `sessionId := idGenerator.…` (interface call only; **never** inline `uuid` in the handler).
  - **Message for ADK:** `msg, err := requestMapper.ToGenAIContent(req.Message)` (or equivalent); handler checks `err` and maps to 400 when the mapper rejects input.
  - Call `runner.Run(ctx, RunParams{UserID: req.UserId, SessionID: sessionId, Message: msg})`.
  - On **immediate** error before writing the response body: respond with **400/500** + `agentapi.ProblemDetails` JSON as appropriate.
  - On success: delegate to the **SSE response driver** with the returned **`RunResult`** (see §4.3). The driver sets headers, emits **`sessionBound`** (using `RunResult.SessionID()`), iterates `RunResult` events (via `Events()` once §4.5 exists), maps each through the **stream-event mapper**, writes **`agent`** / **`error`** SSE as appropriate, then **`done`**—handler does **not** loop over events.

### 4.2 Runner interface (next to consumer)

Per [golang-coding-guide](../../../docs/golang-coding-guide.md), define a **narrow interface** in the same file as the HTTP handler (or adjacent file in [`agentapi`](../../../internal/agentapi)), e.g.:

```go
type agentRunStarter interface {
    Run(ctx context.Context, params RunParams) (*RunResult, error)
}
```

`*AgentRunner` implements this without wrappers if signatures match.

### 4.3 Injectable SSE response driver (component 3)

A **dedicated struct** (name TBD, e.g. `AgentAPISSEWriter` or `SSEResponseDriver`) with a **constructor**, **injected** into `AgentAPIServer`. It is the **single place** that turns a completed **`RunResult`** into a spec-compliant SSE response over `net/http` (reusable for other handlers that produce the same stream shape later).

| Responsibility | Notes |
|----------------|--------|
| **Input** | **`RunResult`** (requires `Events()` from §4.5), plus `http.ResponseWriter` and `context.Context` (for cancellation while iterating). Session id for **`sessionBound`** comes from **`RunResult.SessionID()`**—same value the runner bound—so the handler does not pass a duplicate string unless tests need an override. |
| **Stream / mapping** | **Internally** iterates `RunResult`’s event sequence and uses the **injectable stream-event mapper** (§4.4) to map each `*session.Event` → `agentapi` stream payloads. The mapper stays a **separate, unit-tested** type; the driver **orchestrates** loop + error handling + SSE writes. |
| **HTTP / SSE transport** | Set **`text/event-stream`** headers (`Content-Type: text/event-stream; charset=utf-8`, cache/no-buffer headers as appropriate for SSE). |
| **Framing** | Write `event:` / `data:` lines with JSON for `StreamEvent`, `SessionBoundEvent`, `AgentStreamEvent`, `DoneEvent`, `StreamErrorEvent` as produced by mapping + driver logic. |
| **Flush** | Use `http.Flusher` after each logical event so tests and clients see incremental output. |
| **Order** | Emit **`sessionBound`** first, then zero or more **`agent`** / **`error`**-shaped events per mapper output, then **`done`** on success. Iterator or mapping failures → **`error`** SSE and/or abort per error-handling rules (align with §2). |

**API shape (illustrative):** one entry point such as `StreamAgentRun(ctx context.Context, w http.ResponseWriter, result *RunResult) error` (exact name/signature TBD). Optional lower-level helpers (`writeEvent`, `beginSSE`) may remain **unexported** for tests or for sharing with other transports; keep a **narrow interface** next to the handler if tests need a fake driver.

**Testing:** `runtime/internal/agentapi/sse_*.go` (names TBD) with **dedicated tests**: fake `RunResult` (or iterator) + fake/stub **stream-event mapper** to assert header, event order (`sessionBound` → … → `done`), line format, flush—**without** the full HTTP handler unless in a thin integration test.

**Why `RunResult` as input:** Keeps the handler to **run + delegate**; all **event-order and mapping policy** for the wire lives in one component, while mappers remain **pure** and independently testable.

### 4.4 Two injectable mapper components (grouping and tests)

Mapping is **not** a loose set of package-level functions. Use **two focused structs** (names TBD, e.g. `AgentAPIRequestMapper` and `AgentAPIStreamEventMapper`), each with a constructor. **Request mapper** is injected into **`AgentAPIServer`**. **Stream-event mapper** is injected into the **SSE response driver** (§4.3)—the handler does not call it directly. Interfaces live **next to the consumer** (handler vs driver) unless a second consumer appears.

| Component | Responsibility | Suggested file pair |
|-----------|----------------|---------------------|
| **1 — Request / ADK input mapper** | Validates and maps **inbound** API types to ADK/runtime inputs: `agentapi.UserContent` → `*genai.Content`; optionally centralize validation rules for “empty parts”, role defaults, etc. Returns errors the handler turns into **400**. | e.g. `runtime/internal/agentapi/request_mapper.go` + `request_mapper_test.go` |
| **2 — Stream / API output mapper** | Maps **outbound** `*session.Event` → `agentapi.StreamEvent` (using generated helpers like `FromAgentStreamEvent`, `FromStreamErrorEvent`, etc.): partial text, `turnComplete`, ADK error codes. **No** `http.ResponseWriter` here—pure data mapping for testability. **Used by the SSE driver** while streaming from **`RunResult`**. | e.g. `runtime/internal/agentapi/stream_event_mapper.go` + `stream_event_mapper_test.go` |

**Testing:** Each mapper has **its own** `_test.go` with table-driven cases; the **handler tests** use **fake or stub request mapper** and **fake/stub SSE driver** (which may embed a stub stream-event mapper) only when asserting orchestration (e.g. error propagation), not to re-test mapping rules. Handler tests focus on HTTP status, SSE shape, and delegation to **`RunResult`** streaming.

**Minimal mapping scope (v1):** Request mapper covers the happy path + obvious validation failures; stream mapper covers at least one **partial** and one **final** `agent` path plus error-like events; full ADK parity can grow inside these components later.

### 4.5 `RunResult` access to events

- **Option A (preferred):** Add a method on `*RunResult`, e.g. `func (r *RunResult) Events() iter.Seq2[*session.Event, error]`, exposing the existing iterator for SSE mapping (keeps a single source of truth).
- **Option B:** Change handler to call `LLMRunner` directly for SSE only—duplicates session resolution from `AgentRunner.Run`—**avoid** unless Option A is blocked.

### 4.6 `ContinueAgentRun`

- For this plan: **stub** with **501 Not Implemented** (or **405**—pick one and document) and a short JSON body, so `ServerInterface` is fully implemented and compiles. No streaming logic yet.

### 4.7 Optional `request.Model`

- `AgentRunRequest` includes optional `model`; `RunParams` has **no** model field today. **Decision for v1 minimal:** **Ignore** `model` in the handler and document a TODO, **or** fail fast with 400 “not supported”—choose one in implementation and test it.

---

## 5. Key architectural decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where lives `ServerInterface` impl | [`runtime/internal/agentapi`](../../../internal/agentapi) (package `agentapi`) | Hand-written handler, mappers, SSE driver, and optional id generator live **here**, next to `api.gen.go`. Package `agentapi` imports package `internal` for `AgentRunner`, `RunResult`, `RunParams` (no import cycle: `internal` does not import `agentapi` today). |
| Session id generation | Injectable generator, pattern per [`ident`](../../../../tools/firecrawl/internal/system/ident) | Deterministic ids in tests; no hidden globals. |
| DI style | Constructor + interfaces for runner, id generator, **request mapper**, **SSE response driver** (constructor takes **stream-event mapper**), and optional narrow types for testing | Handler stays thin; driver owns **`RunResult` → SSE**; boundary behavior tested per component. |
| Tests | `httptest`, table/nested `t.Run` | Same patterns as `agentrun_test.go` (faker, testify, `t.Context()`). |
| Generated code | Do not hand-edit `api.gen.go` | Regenerate via `go generate ./internal/agentapi` only when OpenAPI changes. |
| Linting generated API | oapi-codegen [recommends not linting generated code](https://github.com/oapi-codegen/oapi-codegen/tree/main?tab=readme-ov-file#should-i-lint-the-generated-code) | Keep excludes if already present in `golangci` config. |

---

## 6. Uncertainties / follow-ups

- **Auth (`401`):** No auth in minimal slice; returning 401 may be deferred until security design exists.
- **Request body size limits:** Confirm project-wide middleware or add per-handler limit.
- **Model override:** Product decision whether to extend `RunParams` / runner or reject the field.
- **compress-implementation-summaries:** The template references `.context/compress-implementation-summaries.md`; if missing in the repo, confirm the final “compress summaries” step with the team.

---

## 7. Related files

**Existing (read / extend):**

- `runtime/internal/agentapi/api.gen.go` — `ServerInterface`, types, `HandlerFromMux`.
- `runtime/internal/agentapi/openapi.yaml` — contract for SSE and errors.
- `runtime/internal/agentrun.go` — `AgentRunner`, `RunParams`, `Run`.
- `runtime/internal/run_result.go` — `RunResult`; may add `Events()` (or equivalent).
- `runtime/internal/agentapi/api.go` — `go:generate` for codegen.
- `runtime/AGENTS.md` — module conventions.
- `docs/golang-coding-guide.md`, `docs/testing-best-practices.md`.

**New (typical; all under [`runtime/internal/agentapi`](../../../internal/agentapi), package `agentapi`):**

- `server.go` + `server_test.go` (names TBD) — `ServerInterface` implementation + constructor (deps: runner, logger, **id generator**, **request mapper**, **SSE response driver**; the driver itself holds the **stream-event mapper**).
- `request_mapper.go` + `request_mapper_test.go` — injectable component (1), `UserContent` → `genai.Content`.
- `stream_event_mapper.go` + `stream_event_mapper_test.go` — injectable component (2), `session.Event` → `StreamEvent`.
- `sse.go` + `sse_test.go` (names TBD) — injectable component (3), SSE headers + `event:`/`data:` framing + flush.
- Optional: `generator.go` / `mock_generator.go` (or `idgen/` subpackage under `agentapi/`) following `ident` shape — **not** in package `internal` root unless shared beyond the HTTP API.

**Documentation:**

- This plan: `runtime/doc/implementation/agent-api-start-handler/plan-agent-api-start-handler.md`.

---

## 8. Task list (TDD; module completion protocol)

Follow **TDD**: where a task adds behavior, **write failing tests first**, then implement until green. After code changes in `runtime`, run **`make lint`** and **`make test`** from **`runtime/`** (see `runtime/Makefile`). Do not edit `api.gen.go` except via regeneration from `openapi.yaml`.

**Task 1.1: Expose event stream from `RunResult`**

- Add `Events() iter.Seq2[*session.Event, error]` (or equivalent) on `RunResult`, delegating to the existing iterator.
- **Tests first:** extend `run_result_test.go` (or new test file) with a tiny fake iterator; assert `Events()` yields the same sequence.
- Run: `cd runtime && go test ./internal/... -run RunResult -count=1`
- Run `make lint` && `make test` in `runtime/`.

**Task 1.2: Injectable ID generator (pattern from `ident`)**

- Add `Generator` interface + `DefaultGenerator` + test mock in `runtime/internal/agentapi` (same API shape as [`tools/firecrawl/internal/system/ident`](../../../../tools/firecrawl/internal/system/ident) where practical).
- **Tests first:** default produces non-nil ids; mock returns predictable values for `AgentAPIServer` tests.
- Run targeted tests, then full `make test`.

**Task 1.3: Request mapper component**

- Add **`AgentAPIRequestMapper`** (struct + constructor); **tests first** in `runtime/internal/agentapi/request_mapper_test.go`: invalid/empty `UserContent`, happy path to `*genai.Content`.

**Task 1.4: Stream-event mapper component** (before or in parallel with the driver; driver depends on it)

- Add **`AgentAPIStreamEventMapper`** (struct + constructor); **tests first** in `runtime/internal/agentapi/stream_event_mapper_test.go`: table-driven `session.Event` → `StreamEvent` / JSON payload for partial, final, error-like events.
- Pure mapping only—no `http.ResponseWriter`.

**Task 1.5: Injectable SSE response driver**

- Add **`AgentAPISSEWriter`** (or chosen name) (struct + constructor) with **injected stream-event mapper**; **tests first** in `runtime/internal/agentapi/sse_test.go` (name TBD): fake `RunResult` / event iterator + stub mapper, `httptest.ResponseRecorder`, correct `Content-Type`, SSE line format, flush, order **`sessionBound`** → mapped events → **`done`**.
- **API** takes **`RunResult`** (and `ctx`, `w`) and streams end-to-end; mapping stays delegated to the injected mapper.

**Task 1.6: Mock runner + `StartAgentRun` handler skeleton**

- Define `agentRunStarter` (or final name) interface; mock with mockery or hand-written mock matching project norms (see `.context/mockery.md` if used in repo).
- **Tests first:** `httptest.NewRecorder` + `HandlerFromMux` with mock mux: POST `/v1/agent-runs` with valid JSON → **200**, `Content-Type` contains `text/event-stream`, body contains `sessionBound` line and `done` (mock emits zero or one agent events).
- Add cases: malformed JSON → **400**; missing `userId` → **400**.
- Implement `StartAgentRun` until tests pass; **`ContinueAgentRun`** returns **501** (with test asserting status).

**Task 1.7: Wire real `*AgentRunner` (smoke)**

- Optional integration-style test (build tag or separate file if slow): same HTTP path with in-memory session + existing test doubles from `agentrun_test.go` patterns—only if it stays under default test timeout.
- If skipped, document manual smoke steps in a one-line comment or task summary.

**Task 1.8: Documentation**

- Update `runtime/AGENTS.md` **only if** new commands, generate steps, or wiring locations change (e.g. “HTTP handler constructor lives in `internal/agentapi/server.go`”).

**Task 1.9: Completion protocol**

- `cd runtime && make lint` — no errors.
- `cd runtime && make test` — all pass; note coverage if required by team.
- Write per-task brief summaries if the team uses them (path under `runtime/doc/implementation/agent-api-start-handler/`).

**Task 1.10: Compress implementation summaries**

- Follow `.context/compress-implementation-summaries.md` to compress implementation summaries (if that document exists in the branch; otherwise skip or replace with team’s process).

---

## Acceptance criteria

- `var _ agentapi.ServerInterface = (*YourImpl)(nil)` compiles.
- **`POST /v1/agent-runs`** returns SSE with **`sessionBound`** including a new UUID `sessionId`, then streamed **`agent`** events from the runner, then **`done`**, for a valid request with a mock runner.
- **400** on bad input before streaming starts.
- **`ContinueAgentRun`** returns a documented non-success status (e.g. **501**) without panicking.
- **`make lint`** and **`make test`** pass in `runtime/`.
- New logic covered by tests; mocks allow testing without real LLM.
- Session ids come from an **injectable** generator (same **approach** as `tools/firecrawl/internal/system/ident`); **request mapper**, **stream-event mapper** (dependency of the driver), and **SSE response driver** are **injectable components** with **dedicated test files** (handler tests do not duplicate their cases).
