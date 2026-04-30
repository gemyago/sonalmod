# Sonal UI — wireframe (composition, states, behavior)

> **Editing this doc:** Prefer **bullet lists** (and **tables** for state matrices) over long unbroken prose—keep the same detail, but make it scannable and easy to diff.

**Stack**

- Vite + Svelte 5 SPA, `svelte-spa-router`.

**Shell (`Nav` + `<main>`)**

- **Nav — left:** Brand label **Sonalmod** → `/chat`.
- **Nav — center:** **Chat** / **Providers** share the same centered max-width column as `main-inner` on non-chat routes; the nav’s first grid column is content-sized so the brand does not overlap those links on narrow viewports.
- **Nav — right:** **Sign out** (text) **to the left of** the compact **Theme** segmented control; both **end-aligned** in the right margin; only when authenticated.
- **`<main>`:** Centered content wrapper `main-inner` (~800–900px max width) with `Router` inside.
- **`/chat`:** `main-inner` is full width so the session rail can sit at the viewport’s left inset; shell is viewport-height with **no page scrollbar** — only the transcript strip scrolls.
- **Sign out:** `authStore.clearAuth()` (removes refresh token from `localStorage`, clears in-memory session) and `replace('/login')` so the user lands on the public login route without stacking authenticated history entries.
- **Unauthenticated routes:** No `Nav`; stored theme still applies to the global shell on load.

**Theme**

- Compact **Theme** segmented control in `Nav` (Auto / Light / Dark; Lucide icons only; `aria-label` / `title`).
- Default **Auto** (follows `prefers-color-scheme`); choice stored in `localStorage` (`sonal-ui-theme`).

**Chat URLs (behavior)**

- `/chat` without a `sessionId` starts with an empty transcript.
- `/chat/{sessionId}` loads history from the server on open/reload via `readSession`.

**Scope:** This doc is **not** about visual styling (see `DESIGN.md`).

**Auth bootstrap**

- On mount, `App` calls `authStore.tryRestoreSession()` (reads refresh token from `localStorage`, attempts refresh).
- While restoring: `authStore.restoring` is `true` and a loading indicator is shown instead of the shell.
- After restore completes: Router renders and route guards apply.

**Route guarding**

- `/chat` and `/providers`: protected with `svelte-spa-router`’s `wrap` + `conditions`.
- If `authStore.isAuthenticated` is false: `conditionsFailed` → `replace('/login')`.
- `/login` is public.
- Nav is hidden when unauthenticated.

---

## Routes

| Path | Behavior |
| :--- | :--- |
| `/` | `replace('/chat')` on mount; brief status text. |
| `/login` | Login page; username + password form. On success, sets auth tokens and `push('/chat')`. On failure, shows inline error alert. |
| `/chat/:sessionId?` | Chat; optional id in URL after `sessionBound` (`replace`). One route entry so binding the id does not remount the page or abort the stream. |
| `/providers` | Provider configuration management page. Protected (auth required). |

---

## Chat (`/chat/:sessionId?`)

**Layout**

- **Shell:** Two columns below `Nav`: **session sidebar** (~200px) + **main column**; fills viewport height; **no document scroll**.
- **Sidebar position:** Inset from the **left edge of the viewport** (same horizontal inset as global main padding).
- **`SessionList` (rail):**
  - **New chat** — full width of the rail (Lucide `SquarePen` + label; secondary styling; resets state and `push('/chat')`).
  - **Session rows** — scrollable list (title + relative time from `updatedAt`) that grows to use the rail’s height; each row links to `/chat/{sessionId}`; **active** session highlighted from route `sessionId`.
- **≤768px:** Sidebar is a **fixed** panel (narrower than the main column, flush **left** to the viewport), **starts collapsed** off-screen; **Sessions** in the main header toggles open/closed (`sidebar-collapsed` on the shell).
- **Desktop:** Rail’s **right border** spans the **full height** of the chat shell.
- **Main column:** Header (“Chat” page title), then **chat column** in order:
  - **Transcript strip** — flex fills space between header and composer; **overflow-y** only on this strip.
  - **Turn activity** → optional **profile availability banner** (loading, error, or empty) → **composer** (textarea, then **profile** `<select>` left + **Send** right on one row) **pinned to the bottom** of the viewport.
- **Message chrome:** User turns — bordered, raised bubble, **end**-aligned. Assistant turns — plain body text on default surface (no card), including streaming lines.
- **Composer:** Shown **by default** on `/chat` even when the URL has no `sessionId` (implicit new chat). **Enter** submits the message; **Shift+Enter** inserts a newline in the textarea.

**Session list data**

- On mount and after each send stream reaches `done`, the page calls `GET /sessions?limit=50` (`listSessions`) to refresh the sidebar.
- Failures are ignored (best-effort).

| State | When | UI |
| :--- | :--- | :--- |
| New chat (default) | No URL `sessionId` | Sidebar + header + empty transcript + composer (default focus). **New chat** still clears draft and `push('/chat')` when starting from a session or re-clicked. |
| Composer with session | URL has `sessionId` | Sidebar + messages + composer. **New chat** in sidebar resets to `/chat`. |
| Reconnecting | `sessionId` in URL on mount; `readSession` in progress | Streaming UI shown (busy); input disabled until session loads. |
| Streaming | `streamState.busy` | Extra assistant line: live text (incremental `agent` SSE chunks concatenated) or "Thinking…". |
| Error | `runError` | Alert in the turn-activity strip (scroll region), directly above composer. |
| Send off | Empty input or `sendDisabled` | Send disabled; textarea disabled while sending. |
| No selectable profiles | `listModels` succeeded with zero models, or not yet successful | Send disabled; short copy with in-app link to `/providers`. |
| Profile options load error | `listModels` failed | Error alert; Send disabled until a successful load yields at least one selectable profile. |
| Profile options loading | `listModels` in flight | Send disabled; “Loading profiles…” shown above the composer. |

**Session / API**

- No session → `startAgentRun`; else → `continueAgentRun`.
- On `sessionBound`: `replace('/chat/{id}')` — the UI does **not** call `readSession` for that transition; the POST response stream is the only consumer until `done`.
- On `done`: append assistant text to `messages` and clear in-flight turn state.
- Errors from HTTP or stream → `runError`.
- New send or unmount aborts via `AbortController` (hydration vs agent-run scopes are separate).
- All API calls include `Authorization: Bearer <accessToken>` from `authStore`; `userId` is not sent in the body or query (server derives identity via `CallerIdentity`).

**Profile picker**

- On mount: `listModels()` (`GET /models`).
- Until it succeeds with at least one selectable profile: Send disabled.
- If the call fails: error shown; Send stays disabled.
- If it succeeds with an empty list: short message + link to **Providers** (`/providers`) — no implied default model.
- When models exist: `<select>` on the **same row** as **Send** (profile left, Send right).
- Selected value: `localStorage` (`selectedModel`); restored on load if still in the list; otherwise first model selected and persisted.
- Current source of selectable profiles: `listModels()` until dedicated profile listing is wired for the chat page.
- Sends: fully-qualified name (`provider/model-name`) as `model` on `AgentRunRequest` while the list is valid.

**Reconnection (URL has `sessionId` on mount)**

- `readSession` (GET SSE).
- Stream order: `sessionBound` (confirms session) → `sessionStatus` (`active` or `idle`) → zero or more `agent` events (history) → optionally live `agent` if active → `done`.
- **`idle`:** Historical `agent` events folded into the transcript (`user` and `model` from SSE); model chunks use the same incremental aggregation as live runs; turns flush on `turnComplete`, role transitions (user after model), trailing model text on `done`; composer ready when idle.
- **`active`:** Streaming UI while live events arrive; on `done`, streamed turn appended to `messages`.
- Subsequent sends: `continueAgentRun` as usual.

**Tool call blocks**

- Agent turns may include tool calls alongside or instead of text.
- Each: `ToolCallBlock` — collapsible `<details>` / `<summary>`.
- **Collapsed (default):** "Tool call:" + function name (e.g. `workspacefs_write_file`).
- **Expanded:** Formatted JSON for "Arguments" (`args`) and, if present, "Response" (`response`).
- Placement: Below assistant text, or standalone if the turn has no text.
- Same in streaming turn-activity during a run and in committed transcript after `done`.
- Distinct muted-border style vs plain text messages.

**A11y**

- List: `aria-live="polite"`.
- Errors: `role="alert"`.
- Labeled input.

---

## Providers (`/providers`)

**Layout**

- Header ("Providers" heading + "Add Provider" button) → provider list/table → add/edit form (inline, on demand) → delete confirmation dialog.

**Provider list (table)**

- On mount: `listProviders()` loads data.
- Columns: display name (falls back to `name`; technical `name` below display name when set), `type`, `baseUrl`, `apiKeyPreview`, model count (`N models`), Edit and Delete.

| State | When | UI |
| :--- | :--- | :--- |
| Loading | API call in progress | Loading spinner/indicator; list hidden or dimmed. |
| Empty | No providers configured | "No providers configured yet. Add your first provider to get started." message. |
| List | Providers loaded | Table with one row per provider; Edit/Delete buttons per row. |
| Add form | "Add Provider" button clicked | Inline form with fields: `name` (text, required, create-only), `type` (select, create-only, only `openai-compatible`), `displayName` (text, optional), `baseUrl` (url, required), `apiKey` (password, required on create), `models` (dynamic list, see Models in forms below). Save + Cancel buttons. |
| Edit form | "Edit" button clicked on a row | Same form pre-populated; `name` and `type` are read-only. `apiKey` is optional (omit to keep current). `models` pre-populated with existing models. |
| Delete confirm | "Delete" button clicked | Confirmation dialog/prompt before deletion. On confirm, calls `deleteProvider`, refreshes list. |
| Error | API call fails | Alert shown with error message. |

**Models in forms**

- Create and edit: "Models" section with a list of entries.
- Each entry: `name` (required, placeholder `e.g. gpt-4.1`), `displayName` (optional, placeholder `e.g. GPT 4.1`), "✕ Remove model".
- "Add Model" appends a new empty entry.
- Blank `name` entries ignored on submit.
- On update, the models list **replaces** the provider’s existing models.

**API**

- Create → `createProvider` (includes `models` array).
- Edit → `updateProvider` (includes `models` array, replaces existing).
- Delete → `deleteProvider`.
- After each mutating operation: `listProviders()` to refresh the list.
- API key never shown in full — only `apiKeyPreview` (`...XXXX`).
