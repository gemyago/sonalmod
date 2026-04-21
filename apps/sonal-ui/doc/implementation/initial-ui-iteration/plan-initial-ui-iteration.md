# Plan: Initial UI — LLM chat session (SSE)

## 1. Introduction / overview

**Problem:** After framework research ([frameworks-research.md](./frameworks-research.md)), the Sonal UI shell needs a first real feature: start an agent session from the browser, stream the model reply as it arrives, and send follow-up messages on the same session.

**Goal:** On the home route, the user clicks **New chat**, a text composer appears, submitting the first message calls **`POST /agent-runs`**, the UI binds **`sessionId`** from the **`sessionBound`** SSE event, **updates the browser URL** (hash) to include that id, renders streamed **`agent`** events incrementally, and after **`done`** allows further sends via **`POST /sessions/{sessionId}/agent-runs`**. After a **full page refresh** while the hash still contains the session id, the client **restores the same `sessionId`** so follow-up messages continue to target that session. The implementation must be **reactive to SSE** (no “wait until complete”-only UX).

**API contract:** [runtime/internal/agentapi/openapi.yaml](../../../../../runtime/internal/agentapi/openapi.yaml) — JSON request body `AgentRunRequest` (`userId`, `message.parts[]`, optional `model`), response `text/event-stream` with JSON `data` lines discriminated by `event`: `sessionBound`, `agent`, `error`, `done`.

**Non-goals (this iteration):** User accounts, **server-backed transcript replay** (no history list API in scope), conversation history sidebar, model picker UI, auth UI (handle `401` gracefully if present), or production deployment hardening beyond configurable base URL. **Note:** Reload restores **`sessionId` from the URL** only; **message text shown before refresh is not automatically restored** unless persisted client-side in a follow-up task (see §2).

---

## 2. Business logic

1. **Idle / “new conversation”:** No active `sessionId` in app state **and** URL hash does not encode a session (see below). User sees a **New chat** control; optional short helper copy.
2. **URL ↔ session binding (hash router):** Use **`svelte-spa-router`** with a route that carries **`sessionId`**, e.g. `#/chat` (no session) vs `#/chat/<sessionId>` (bound). **On `sessionBound`:** set `sessionId` in state and **update the hash** with `history.replaceState` or the router’s navigation API so the path becomes `#/chat/<sessionId>` (prefer **replace** on first bind so “Back” does not step through a no-id URL unless product wants **push**). **On initial load / hash change:** if the hash contains a `sessionId`, **hydrate** app state from it so **refresh** keeps the same id for **`continue`** calls. **New chat** navigates to `#/chat` (no id) and clears local UI state.
3. **Reveal composer:** Clicking **New chat** shows a **text box** (and submit affordance). First submit sends **start run** with the trimmed message; empty submits are ignored. If the URL already has a `sessionId` (after refresh), show the composer **without** requiring “New chat” again — user can send a **continue** message directly (subject to server session still existing).
4. **Request shape:** `POST` with JSON `{ "userId": "<non-empty>", "message": { "parts": [{ "text": "..." }] } }`. `userId` must satisfy the API; use a **stable dev default** from `VITE_*` (e.g. `VITE_AGENT_USER_ID`) or a generated id stored in `sessionStorage` — **document the choice** in code comments only if non-obvious.
5. **SSE handling:** Use **`fetch`** with `Accept: text/event-stream` (or default) and read **`response.body`** with a **TextDecoder** (and a small parser), because **`EventSource` does not support POST** with a JSON body. Parse standard SSE frames (`event:` optional, one or more `data:` lines, blank line between events).
6. **Stream events:**
   - **`sessionBound`:** Store `sessionId`, **sync the hash** (see §2.2), for subsequent `continue` calls.
   - **`agent`:** Update the **current assistant turn** display from `content.parts[].text` and flags `partial` / `turnComplete` (see uncertainties).
   - **`error`:** Show a clear error state; stop treating the run as in progress.
   - **`done`:** Mark the current assistant turn as finished; **re-enable** the composer for the next user message (unless starting a brand-new chat).
7. **Follow-up:** After the first turn completes (`done`), **same composer** sends **`POST /sessions/{sessionId}/agent-runs`** with the same JSON shape. **New chat** navigates to `#/chat` (no id), clears local UI state, and **does not** call the server to delete the old session unless an API exists later.
8. **Concurrency:** Disable send (or queue) while a run is in progress to avoid overlapping streams on one session unless product later requires otherwise.

---

## 3. High-level architecture

| Layer | Responsibility |
| --- | --- |
| **Config** | `VITE_AGENT_API_BASE_URL` — origin (or path) for agent API; no secrets. |
| **HTTP + SSE** | Pure TS module(s): build `fetch` URLs, POST JSON, parse SSE stream → typed events (narrow union by `event` field). |
| **Chat state** | Svelte runes (`$state`): messages, current streaming assistant buffer, `sessionId`, `busy`, `error`; **kept in sync** with hash segment when bound. |
| **Routing** | **`#/chat`** and **`#/chat/:sessionId`** (or equivalent) so **refresh** restores `sessionId`; default route may redirect `/` → `#/chat`. |
| **UI** | Chat page (or small subcomponents): New chat, message list, composer; minimal CSS using existing CSS variables (`app.css`). |

**Optional:** Vite **`server.proxy`** in dev maps e.g. `/agent-api` → runtime origin to avoid CORS during local development; **document** in `.env.example` that the base URL can be relative (`''`) when proxying.

---

## 4. Detailed architecture (files)

### 4.1 Types and API client

- **`src/lib/agentapi/types.ts`** — TypeScript types mirroring the OpenAPI shapes needed on the client (`AgentRunRequest`, discriminated stream payloads for `sessionBound`, `agent`, `error`, `done`). Avoid importing generated Go code; keep in sync manually or with a future codegen step.
- **`src/lib/agentapi/sse.ts`** (or `parseAgentSseStream.ts`) — **Incremental** parser: `AsyncIterable<string>` or `ReadableStream` → async iterator of `{ eventName?: string; data: string }`, then `JSON.parse` each `data` line and narrow by `event` field. Unit-testable without network.
- **`src/lib/agentapi/client.ts`** — `startAgentRun(baseUrl, body, signal)` and `continueAgentRun(baseUrl, sessionId, body, signal)` returning `ReadableStream<Uint8Array>` or `Response` for the caller to pipe into the SSE parser. **Params object** if more than three arguments.

### 4.2 UI

- **`src/pages/Chat.svelte`** (and/or keep **`Home.svelte`** as thin wrapper) — Orchestrates: New chat → show composer; on submit call client + parser; bind streamed text to the view; **read `sessionId` from route params** on mount and when the hash changes.
- **`src/components/`** — Optional split: `ChatComposer.svelte`, `MessageList.svelte` if the page grows too large; keep **one screen** unless clarity suffers.

### 4.3 App shell

- **`src/App.svelte`** — Route map includes **`/chat`** and **`/chat/:sessionId`** (hash); **optional:** map **`/`** to the same chat view or redirect to **`/chat`**. Update **App.test.ts** for hash URLs and **navigation after `sessionBound`** (e.g. `window.location.hash` or router helper).
- **`src/components/Nav.svelte`** — Point “Home” / primary link to **`#/chat`** (or `/` if redirected) so it matches the new flow.

### 4.4 Config and env

- **`.env.example`** — Add `VITE_AGENT_API_BASE_URL` (e.g. `http://localhost:<port>` or empty for proxy), `VITE_AGENT_USER_ID` (or equivalent).
- **`vite.config.ts`** — Optional `server.proxy` for dev; only if the team agrees on a single convention (document in plan task list).
- **`.env.test`** — Set `VITE_AGENT_API_BASE_URL` and `VITE_AGENT_USER_ID` for deterministic tests.

### 4.5 Tests (TDD)

- **Unit tests** for SSE parsing (fixture strings covering multi-line `data`, `event:` lines, multiple events in one chunk).
- **Unit tests** for a small reducer or pure function that merges **`agent`** events into “display text for current turn” **given** documented assumptions (see §6).
- **Component test** (optional but valuable): mock `fetch` to return a static `ReadableStream` of SSE bytes and assert the UI shows **streaming text** updates (e.g. partial then final).

---

## 5. Key architectural decisions

| Decision | Choice | Rationale |
| --- | --- | --- |
| SSE transport | **`fetch` + stream parser** | `EventSource` cannot POST JSON bodies for `/agent-runs`. |
| API base URL | **`VITE_AGENT_API_BASE_URL`** | No hardcoded host; matches existing `VITE_*` pattern in [doc/architecture.md](../../architecture.md). |
| Styling | **Existing global CSS** | No new UI kit in this iteration; frameworks research reserved for a later polish pass. |
| Session in URL | **`#/chat/:sessionId`** (hash) | Static hosting–friendly; **refresh** restores id for **`continue`**; transcript not replayed unless stored client-side later. |
| Hash routing | **Extend** | Add chat routes; keep **`/about`** etc. as today. |

---

## 6. Uncertainties

1. **Partial vs cumulative text:** OpenAPI describes `partial` as incremental; example shows a short partial then a **longer final** string. Confirm whether each `agent` event’s `content.parts` is **cumulative for the turn** or **delta-only**; the UI should match **runtime behavior** (inspect `server_test` / manual run against the real runner). If deltas: concatenate; if cumulative: replace display from latest event.
2. **Runtime base path:** `HandlerFromMux` uses empty `BaseURL` today (`POST /agent-runs` at mux root). If a future `cmd` mounts the mux under a prefix, **only** `VITE_AGENT_API_BASE_URL` needs to include that prefix — document when wiring production.
3. **Auth:** `401` responses are **not** SSE; plan for a simple error message if the fetch fails before streaming.
4. **CORS:** Browser will block cross-origin POSTs without server CORS headers; **dev proxy** or **same-origin** deployment is required for local testing.
5. **Refresh vs server session:** The URL **only** persists the id client-side. If the **runtime** has dropped the session (restart, TTL), **`continue`** may return **404** or error — show a clear message and offer **New chat**.

---

## 7. Related files

**Existing (read / update):**

- `apps/sonal-ui/src/App.svelte`, `src/pages/Home.svelte`, `src/App.test.ts`
- `apps/sonal-ui/vite.config.ts`, `.env.example`, `.env.test`
- `apps/sonal-ui/doc/architecture.md` — update **only if** new env vars or conventions are introduced (per project rules)
- `runtime/internal/agentapi/openapi.yaml` — reference only unless the API contract changes

**New (typical):**

- `apps/sonal-ui/src/lib/agentapi/types.ts`
- `apps/sonal-ui/src/lib/agentapi/sse.ts` (or `parseAgentSseStream.ts`)
- `apps/sonal-ui/src/lib/agentapi/client.ts`
- `apps/sonal-ui/src/lib/agentapi/sse.test.ts` (and/or `*.spec.ts`)
- Optional: `src/components/ChatComposer.svelte`, `MessageList.svelte`
- `apps/sonal-ui/doc/implementation/initial-ui-iteration/summary-task-*.md` — per task during implementation

---

## 8. Task list

Implementation follows **TDD** where practical: write failing tests for **SSE parsing** and **display merge logic** first, then implement. After each task, keep the **app buildable** (`npm run build` from `apps/sonal-ui`). **Module completion:** from repo root, **`make lint`** and **`make test`** before declaring the work done ([AGENTS.md](../../../AGENTS.md)).

---

**Task 1.1: SSE stream parser (tests first)**

- Add `src/lib/agentapi/sse.ts` (or agreed name) with a pure async parser from `ReadableStream`/`Uint8Array` chunks to parsed JSON payloads.
- Write failing tests in `src/lib/agentapi/sse.test.ts` using **string fixtures** copied from `openapi.yaml` examples (sessionBound → agent partial → agent final → done).
- Implement until tests pass (handle chunked TCP splits across `\n\n` boundaries).
- Run `npm run test:run -- src/lib/agentapi/sse.test.ts` (or `vitest run` path).
- Write summary to `doc/implementation/initial-ui-iteration/summary-task-1.1.md`.
- All checks from completion protocol must be passed for touched code (root `make lint` / `make test` when committing this module).

---

**Task 1.2: Typed stream events and merge helper**

- Add `types.ts` with request/response types aligned with OpenAPI.
- Add a pure function, e.g. `applyAgentStreamEvent(state, event) → state`, covered by tests **after** confirming cumulative vs delta behavior (Task 1.1 / manual spike); start with **OpenAPI example behavior** (latest snapshot wins if examples show full text in last chunk).
- **Tests:** `error` clears/fails; `sessionBound` sets id; `done` flips idle.
- Run targeted Vitest, then full `make test` from repo root.
- Write summary to `summary-task-1.2.md`.

---

**Task 1.3: HTTP client wrapper**

- Add `client.ts`: `startAgentRun` / `continueAgentRun` using `fetch`, `method: 'POST'`, `headers: { 'Content-Type': 'application/json' }`, `body: JSON.stringify(...)`.
- **Tests:** mock `global.fetch` to return a `Response` with a `ReadableStream` body from fixture strings; assert parser is invoked (or integration test with parser).
- **AbortSignal:** accept optional `AbortSignal` for cancel-on-unmount (optional in first iteration).
- Write summary to `summary-task-1.3.md`.
- Run root `make lint` and `make test`.

---

**Task 1.4: Chat page UI + hash route for `sessionId`**

- Add routes **`/chat`** and **`/chat/:sessionId`** (hash); wire **Chat** view (replace or refactor **`Home.svelte`** as decided).
- On **`sessionBound`**, navigate so the hash becomes **`#/chat/<sessionId>`** (prefer **`replace`**-style navigation to avoid cluttering history).
- On load, **parse `sessionId` from the route**; if present, **use `continue`** for sends. Use **`start`** only when the user began from **`#/chat`** without an id (until `sessionBound` fills the id).
- Replace placeholder content with **New chat** → composer → message list + streaming assistant area.
- Wire `VITE_AGENT_API_BASE_URL` and `VITE_AGENT_USER_ID` (or chosen id strategy).
- **Manual:** verify against a running runtime; **refresh** on `#/chat/<id>` and confirm **continue** still targets the same id (and document §2 limitation if transcript is empty after reload).
- Add/update **App.test.ts** (and any new component tests): default view, **hash with session param** if testable without full E2E.
- Write summary to `summary-task-1.4.md`.
- Run root `make lint` and `make test`.

---

**Task 1.5: Dev ergonomics (optional but recommended)**

- Add **Vite `server.proxy`** and document in `.env.example` that `VITE_AGENT_API_BASE_URL=/agent-api` (or similar) works with proxy target.
- **If** `doc/architecture.md` env section should list new vars, update it in the same task.
- Write summary to `summary-task-1.5.md`.
- Run root `make lint` and `make test`.

---

**Task 1.6: Compress implementation summaries**

- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to produce `implementation-summary.md` from `summary-task-*.md` and remove per-task summaries.
- **Note:** That instruction uses sub-agents for extraction; if your environment cannot run sub-agents, follow the **Pre Condition** in that file and adapt manually while preserving the required output format.

---

## References

- OpenAPI (repo root): `runtime/internal/agentapi/openapi.yaml`
- UI stack: `apps/sonal-ui/doc/architecture.md`
- Framework research: [frameworks-research.md](./frameworks-research.md)
