# Plan: OpenAPI TypeScript + openapi-fetch (sonal-ui)

## 1. Introduction / overview

**Goal:** Adopt the stack described in [openapi-typescript-and-fetch-distilled.md](./openapi-typescript-and-fetch-distilled.md): **`openapi-typescript`** for dev/CI codegen (OpenAPI → `paths` / `components` types) and **`openapi-fetch`** at runtime for typed HTTP calls. **No** Hey API, Orval, or OpenAPI Generator.

**Problem solved:** `apps/sonal-ui/src/lib/agentapi/types.ts` is manually kept in sync with [runtime/internal/agentapi/openapi.yaml](../../../../../runtime/internal/agentapi/openapi.yaml). That drifts when the spec changes. Generated types and a single typed client reduce duplication and catch contract mismatches at build time.

**Non-goals (this iteration):** Replacing SSE parsing with a different library; changing the Go server or `oapi-codegen` workflow in `runtime/internal/agentapi`.

---

## 2. Business logic (contract, in words)

- **Source of truth:** The same OpenAPI 3.1 YAML the runtime uses ([runtime/internal/agentapi/openapi.yaml](../../../../../runtime/internal/agentapi/openapi.yaml)).
- **HTTP surface:** Two `POST` operations (`/agent-runs`, `/sessions/{sessionId}/agent-runs`) with JSON bodies; **`200`** responses are **`text/event-stream`** (SSE), not JSON. Per the distilled doc, **`openapi-fetch` does not replace streaming**: callers must use the returned **`response`** (and existing `parseAgentSseJsonStream` / SSE pipeline) for the body, not assume `data` is a parsed JSON payload for those routes.
- **Error bodies:** Non-2xx responses may expose `application/problem+json` (`ProblemDetails`); typed error handling should align with generated `components` where practical.
- **SSE event JSON:** Each `data:` line is JSON matching **`StreamEvent`** and its `oneOf` branches in the spec (`SessionBoundEvent`, `AgentStreamEvent`, `StreamErrorEvent`, `DoneEvent`). The parser must **not** keep parallel hand-written event interfaces: it should use the **generated** schema types for the parsed payload (see §4.6).

---

## 3. High-level architecture

| Piece | Role |
| --- | --- |
| **`openapi-typescript` (dev)** | Invoked by **`make generate-api`** from **`apps/sonal-ui`** → emits one generated module (e.g. `.d.ts` or `.ts`) under `src/lib/…`. |
| **`openapi-fetch` (runtime)** | `createClient<paths>({ baseUrl, … })` — shared module exporting the client factory or singleton. |
| **Makefile (`apps/sonal-ui/Makefile`)** | **Only** entry for codegen: **`make generate-api`** runs **`openapi-typescript`** (input spec, output path, flags in the Makefile); **`make check-api`** runs the same **`openapi-typescript`** line **plus** **`--check`**. No separate npm scripts for these. |
| **SSE parser (`sse.ts`)** | Parsing stays the same algorithmically; **types** change: `SseJsonEvent` / yielded events use the generated **`StreamEvent`** (and related **`components.schemas`**) types for **`payload`**, replacing **`unknown`** and any duplicate definitions in `types.ts`. Callers (e.g. `Chat.svelte`) consume **`StreamEvent`**-shaped data end-to-end. |

---

## 4. Detailed architecture

### 4.1 Spec input path

- From `apps/sonal-ui`, the spec is **`../../runtime/internal/agentapi/openapi.yaml`** (repo-root-relative: `runtime/internal/agentapi/openapi.yaml`).
- Codegen must not copy the YAML into sonal-ui; a single file avoids divergence.

### 4.2 Generated output location

- Place generated artifacts under `src/lib/agentapi/` (or `src/lib/api/`) with a clear name, e.g. `agentapi.generated.d.ts` / `agentapi.generated.ts`, **or** follow the distilled convention `v1.d.ts` inside `src/lib/api/`. Pick one naming scheme and document it in `AGENTS.md`.
- Commit the generated file(s) so CI and offline builds work without running codegen first (same idea as Go `go generate` outputs being committed unless policy says otherwise).

### 4.3 `package.json`

- **dependencies:** `openapi-fetch`
- **devDependencies:** `openapi-typescript` (installed so **`npx openapi-typescript`** / **`node_modules/.bin/openapi-typescript`** work when the Makefile runs from `apps/sonal-ui`).
- No **`generate:api`** / **`check:api`** npm scripts — codegen is Makefile-only.

### 4.4 `apps/sonal-ui/Makefile`

- Add **`.PHONY`** targets **`generate-api`** and **`check-api`** with recipes that invoke **`openapi-typescript`** directly (e.g. via **`npx`** or **`$(CURDIR)/node_modules/.bin/openapi-typescript`**).
- Encode in the Makefile: spec path **`../../runtime/internal/agentapi/openapi.yaml`**, output file path, and shared flags (optional: **`--read-write-markers`** later). **`check-api`** repeats the same invocation **plus** **`--check`**.
- Optionally add a **`help`** target listing these (nice for juniors; optional).

### 4.5 Runtime client module

- New or refactored module (e.g. `client.ts`): **`createClient<paths>({ baseUrl: … })`** where base URL comes from **`import.meta.env.VITE_*`** (existing or new env var), consistent with [vite-env.d.ts](../../src/vite-env.d.ts) augmentation.
- Replace raw `fetch` for **start/continue** runs with **`client.POST(…)`** (or the correct path/method per generated `paths`), but **for SSE success responses**, obtain **`ReadableStream`** from **`response.body`** per current behavior (see distilled “SSE / streaming” section).
- Keep **`joinUrl`** exported if tests depend on it, or narrow exports after tests move to MSW against absolute URLs.

### 4.6 SSE parser and generated event types

- The OpenAPI spec already defines **`StreamEvent`** and event variants; **`openapi-typescript`** emits corresponding **`components.schemas`** types.
- Update **`sse.ts`** so exported types are wired to those generated types:
  - Replace **`payload: unknown`** on **`SseJsonEvent`** (and the async generator’s yield type) with the generated **`StreamEvent`** type (exact import path depends on generator output, e.g. **`components['schemas']['StreamEvent']`** or a named export).
  - **`JSON.parse`** returns **`unknown`**: cast or assign to **`StreamEvent`** at the boundary (same as today’s implicit trust of server JSON). If the generated union is awkward for **`oneOf`/discriminator**, document a single narrow helper (e.g. by **`event`** field) that still returns **`StreamEvent`**.
- **Goal:** one source of truth for event shapes — the spec → generated types → parser public API. No duplicate **`SessionBoundEvent`**, **`AgentStreamEvent`**, etc. in hand-written `types.ts` once generated types are available.

### 4.7 Types consolidation (`types.ts` and consumers)

- Delete or shrink manual duplicates: import **`AgentRunRequest`** and any remaining shared types from the generated module; **remove** event interfaces superseded by §4.6.
- Ensure **SSE parser tests** (`sse.test.ts`) still use string fixtures; assert parsed payloads are assignable to **`StreamEvent`** (or match expected fixture objects typed with generated types).

### 4.8 Lint / CI alignment

- **Decision to make in implementation:** Either extend **`make lint`** in sonal-ui to run **`check-api`** after `npm run lint`, **or** document that **`make check-api`** must run in CI before merge. Recommended: include **`check-api`** in the same CI path as root **`make lint`** (e.g. sonal-ui `lint` depends on `check-api`, or root invokes both). Pick one approach and update **`apps/sonal-ui/AGENTS.md`** accordingly.

---

## 5. Key architectural decisions

1. **Single spec file** in `runtime/…` — no duplicate YAML in sonal-ui.
2. **Makefile is the only documented entry** for codegen (`make generate-api`, `make check-api`); the **`openapi-typescript`** command line (paths and flags) lives in **`apps/sonal-ui/Makefile`**.
3. **SSE remains** outside “happy path JSON `data`” for streaming endpoints; use **`response`** from openapi-fetch when calling those operations.
4. **Stream events are spec-typed end-to-end:** generated **`StreamEvent`** types drive **`sse.ts`** exports and consumers; no parallel manual event interfaces.
5. **TDD:** New or refactored client behavior gets Vitest coverage first (MSW or injected `fetch`), per the distilled doc and module conventions (`@faker-js/faker` for sample data).

---

## 6. Uncertainties

- **Exact `openapi-typescript` output** for `oneOf` / discriminator (`StreamEvent`): verify generated types match runtime JSON; adjust exports or use type guards if the generator’s output is awkward.
- **`openapi-fetch` + `text/event-stream`:** Confirm the library’s behavior for non-JSON 200 bodies; if it mis-parses, rely strictly on **`response`** for stream consumption (document in code comments).
- **Lint time:** Running `openapi-typescript --check` on every `make lint` adds seconds; if too slow, split into **`make ci`** vs local **`make lint`** (document clearly).

---

## 7. Related files

**Existing**

- [apps/sonal-ui/Makefile](../../../Makefile)
- [apps/sonal-ui/package.json](../../../package.json)
- [apps/sonal-ui/src/lib/agentapi/client.ts](../../../src/lib/agentapi/client.ts)
- [apps/sonal-ui/src/lib/agentapi/types.ts](../../../src/lib/agentapi/types.ts)
- [apps/sonal-ui/src/lib/agentapi/sse.ts](../../../src/lib/agentapi/sse.ts)
- [runtime/internal/agentapi/openapi.yaml](../../../../../runtime/internal/agentapi/openapi.yaml)
- [apps/sonal-ui/AGENTS.md](../../../AGENTS.md)
- Root [Makefile](../../../../../Makefile) (if sonal-ui lint chain changes)

**New / updated (expected)**

- Generated: e.g. `src/lib/agentapi/*.generated.d.ts` (exact name per implementation)
- Refactored: `client.ts`, `types.ts` (possibly renamed to `stream-types.ts` or merged)
- [apps/sonal-ui/Makefile](../../../Makefile) — new targets
- [apps/sonal-ui/package.json](../../../package.json) — deps (`openapi-fetch`, `openapi-typescript`); no api codegen npm scripts
- [apps/sonal-ui/AGENTS.md](../../../AGENTS.md) — document `make generate-api` / `make check-api` and when to run them

---

## 8. Task list

Follow **TDD** where logic changes: write failing tests first, implement, then green. After each substantive task, keep the module **buildable** (`npm run build` / `make lint` as appropriate). Module completion: **`make lint`** and **`make test`** from **`apps/sonal-ui`**, and root **`make lint`** / **`make test`** per [AGENTS.md](../../../AGENTS.md).

**Task 1.1: Dependencies**

- Add `openapi-fetch` (runtime) and `openapi-typescript` (dev) to `package.json`; run **`npm install`** from `apps/sonal-ui` and commit **`package-lock.json`**.
- Do not add npm scripts for codegen.

**Task 1.2: Makefile targets**

- Extend **`apps/sonal-ui/Makefile`**: **`generate-api`** and **`check-api`** call **`openapi-typescript`** directly (same flags except **`check-api`** adds **`--check`**); document **`make generate-api`** / **`make check-api`** in **`AGENTS.md`**.
- Verify **`make -C apps/sonal-ui generate-api`** produces the artifact locally.

**Task 1.3: Initial codegen commit**

- Run **`make generate-api`**, commit generated file(s). Run **`make check-api`** — must pass.

**Task 2.1: Typed client module (tests first)**

- Write failing Vitest tests for the new client behavior (e.g. MSW handlers for `POST /agent-runs` and continue path, assert **`response`** usable for stream stub, error paths return expected shapes). Use **`@faker-js/faker`** for variable payloads per module rules.
- Implement **`createClient<paths>(…)`** wrapper and refactor **`startAgentRun` / `continueAgentRun`** to use it while preserving **`Response`** for SSE consumption.
- Run **`make test`** in `apps/sonal-ui`; fix until green.

**Task 2.2: Consolidate types and wire SSE parser to generated `StreamEvent`**

- **`sse.ts`:** Type **`SseJsonEvent`** / **`parseAgentSseJsonStream`** yields using the generated **`StreamEvent`** (see §4.6); remove **`unknown`** for SSE payloads where the spec defines the shape.
- **`types.ts` / `Chat.svelte`:** Drop hand-written event interfaces; import event types from the generated module (or re-export from a small **`agentapi/types.ts`** barrel if needed).
- Update **`sse.test.ts`**; **`make test`** green.

**Task 3.1: Lint pipeline and env typing**

- Decide and implement: **`check-api`** as part of **`make lint`** vs separate CI step; update root flow if needed.
- Augment **`vite-env.d.ts`** for any new **`VITE_*`** variables used by the client.

**Task 3.2: Documentation and wireframe**

- If user-visible behavior or configuration changes, update [ui-wireframe.md](../../../ui-wireframe.md); otherwise skip with note in PR.

**Task 4.1: Completion protocol**

- From repo root: **`make lint`** and **`make test`** — all pass.
- Update **`apps/sonal-ui/AGENTS.md`** if commands or workflows changed (required).

**Task 5.1: Compress implementation summaries**

- Follow [.context/compress-implementation-summaries.md](../../../../../.context/compress-implementation-summaries.md) to compress per-task summaries into **`implementation-summary.md`** after implementation tasks complete.

---

## Reference

- Distilled stack guide: [openapi-typescript-and-fetch-distilled.md](./openapi-typescript-and-fetch-distilled.md)
- Upstream docs: [openapi-ts.dev](https://openapi-ts.dev/), [openapi-fetch](https://openapi-ts.dev/openapi-fetch/)
