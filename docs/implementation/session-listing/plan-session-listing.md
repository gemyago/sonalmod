# Plan: Session Listing

## 1. Introduction / Overview

Currently there is no way to list or browse previous sessions. Users can only access a session by directly navigating to `/chat/{sessionId}` — there is no discovery mechanism. This plan adds the ability to list sessions with lightweight metadata and display them in the UI, allowing users to find and resume past conversations.

**Goal:** Enable session listing via a new API endpoint and display sessions in a sidebar on the Chat page. Sessions get human-readable titles — either via LLM summarization (when a summarization model is designated via provider config UI) or by truncating the first user message.

## 2. Business Logic

- When a session is created (via `POST /agent-runs`), session metadata (ID, user, timestamps) is persisted in a lightweight metadata store separate from the full ADK session storage. The metadata `Save` operation uses **upsert** semantics — if the record already exists it is updated, if not it is created. This ensures that even if the initial `Create`-time metadata save failed, a subsequent `AppendEvent` will recover by creating the record.
- Session titles are generated through a `Summarizer` abstraction:
  - **Default (no summarization model designated):** The first user message text (truncated to ~50 chars at a word boundary) is used as the session title.
  - **LLM summarization (when a model is designated):** A designated summarization model generates a concise title (max ~8 words, ~50 chars) from the first user message. On LLM error, the summarizer internally falls back to truncation — callers always get a usable title.
  - The summarization model is designated per-provider in the UI: a checkbox on the model configuration form marks a model for summarization. If multiple models are checked, the first one found is used. A non-disruptive hint guides users to prefer fast, inexpensive models.
- Title is **obligatory** at the API level — the backend always resolves a title. For sessions that have no stored title (e.g. brand-new sessions with no messages yet, or legacy sessions predating this feature), the API mapper applies a fallback: `"Session <created-date>"` (e.g. `"Session Apr 16 14:30"`). **Listing never triggers LLM calls** — the fallback is purely string-based.
- The `GET /sessions` endpoint returns a **paginated** list of session metadata for the authenticated user, sorted by `updatedAt` descending (most recent first). Pagination uses `limit` (required, no default, max 100) and `offset` (optional, defaults to 0).
- The UI displays a session sidebar on the Chat page. Each entry shows the title and a relative timestamp. Clicking an entry navigates to `/chat/{sessionId}`.
- Metadata `updatedAt` is refreshed each time an event is appended, so active sessions sort to the top.

## 3. High-Level Architecture

```
┌──────────────────┐  GET /sessions?limit=N    ┌──────────────────────┐
│   sonal-ui       │ ◄────────────────────────►│  runtime HTTP API     │
│  (SessionList    │                           │  (AgentAPIServer)     │
│   sidebar)       │                           └─────────┬────────────┘
└──────────────────┘                                     │
                                                         ▼
                                                ┌────────────────────┐
                                                │  BackgroundRunner   │
                                                │  (delegates)        │
                                                └────────┬───────────┘
                                                         │
                                                         ▼
                                                ┌────────────────────┐
                                                │  Runner (public)    │
                                                │  ListSessions()     │
                                                └────────┬───────────┘
                                                         │
                                                ┌────────┴───────────┐
                                                ▼                    ▼
                                       ┌──────────────┐    ┌─────────────────────┐
                                       │ session.Service │  │ SessionMetadataStore │
                                       │ (ADK, unchanged) │  │ (NEW — file or DB)  │
                                       └──────────────┘    └─────────────────────┘

                                       ┌─────────────────────────┐
                                       │ Summarizer              │
                                       │ (truncating or LLM)     │
                                       └─────────────────────────┘
```

Components involved:
1. **`SessionMetadataStore`** (new, `runtime/internal/`) — lightweight store for session metadata (title, timestamps). Two implementations: file-based and database-backed. All saves use upsert semantics.
2. **`Summarizer`** (new, `runtime/internal/`) — general-purpose text summarization interface. Two implementations: `TruncatingSummarizer` (default, 50 chars) and `LLMSummarizer` (dynamically resolves designated LLM model from provider config, falls back to truncation on error). Reusable for future summarization needs beyond session titles.
3. **`sessionServiceDecorator`** (new, `runtime/internal/`) — wraps `session.Service` + `SessionMetadataStore` + `Summarizer`. Intercepts `Create`, `AppendEvent`, and `Delete` to keep metadata in sync automatically.
4. **`AgentRunnerFactory` / `Runner` / `BackgroundRunner`** — extended with `ListSessions` method.
5. **`AgentRunner` interface** (public, `runtime/agent/`) — extended with `ListSessions`.
6. **Provider config** (`ModelConfig`, OpenAPI, UI) — `Summarization` boolean added to model config. UI checkbox with hint.
7. **OpenAPI spec + generated code** (`runtime/internal/agentapi/`) — new `GET /sessions` endpoint with pagination.
8. **`AgentAPIServer`** — new `ListSessions` handler with title fallback resolution.
9. **UI: `SessionList.svelte`** (new component), `Chat.svelte` layout update, `Providers.svelte` checkbox, API client extension.

## 4. Detailed Architecture

### 4.1 Session Metadata Types (`runtime/internal/session_metadata.go` — new file)

```go
type SessionMetadata struct {
    SessionID string
    AppName   string
    UserID    string
    Title     string
    CreatedAt time.Time
    UpdatedAt time.Time
}

type ListSessionMetadataParams struct {
    AppName string
    UserID  string
    Limit   int // required — max number of results to return (1–100)
    Offset  int // optional — number of results to skip (default 0)
}

type ListSessionMetadataResult struct {
    Sessions []SessionMetadata
    Total    int // total count of sessions matching the query (for pagination)
}

// SessionMetadataStore persists lightweight session metadata.
// All Save operations use upsert semantics: create if not exists, update if exists.
type SessionMetadataStore interface {
    Save(ctx context.Context, metadata SessionMetadata) error
    List(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)
    Delete(ctx context.Context, appName, userID, sessionID string) error
}
```

### 4.2 Summarizer Interface and Truncating Implementation (`runtime/internal/summarizer.go` — new file)

```go
// Summarizer summarizes text into a concise form.
// This is a general-purpose abstraction, reusable beyond session titles.
type Summarizer interface {
    Summarize(ctx context.Context, text string) (string, error)
}
```

**`TruncatingSummarizer`** (same file):
- Truncates `text` to 50 characters at a word boundary.
- Appends `"..."` if truncated.
- Always succeeds (never returns error).
- Used as default when no summarization model is designated and as internal fallback within `LLMSummarizer`.

### 4.3 LLM Summarizer (`runtime/internal/llm_summarizer.go`)

```go
type LLMSummarizer struct {
    providerService ProvidersConfigService
    modelsLocator   *ModelsLocator
    fallback        Summarizer // *TruncatingSummarizer
    logger          *slog.Logger
}
```

Behavior on each `Summarize(ctx, text)` call:
1. Call `providerService.List(ctx)` to get all providers.
2. Scan providers' `Models` for the first model with `Summarization: true`.
3. If no model found → delegate to `fallback.Summarize(ctx, text)`.
4. If found → call `modelsLocator.ResolveModel(ctx, "providerName/modelName")` to get the `model.LLM`.
5. Use the LLM to generate a title with a prompt: *"Generate a concise title (max 8 words, max 50 characters) for a conversation that starts with: \<text truncated to 200 bytes (same algorithm as TruncatingSummarizer)\>"*.
6. If the LLM call fails → log error, delegate to `fallback.Summarize(ctx, text)`.
7. Return the result.

**Dynamic resolution**: The summarizer checks provider config on every call, so newly designated models take effect immediately without app restart. Since title generation only happens once per session (on first user message), the overhead of scanning providers is negligible.

### 4.4 Provider Config Changes for Summarization

**Go struct** (`runtime/internal/providers_config.go`):
```go
type ModelConfig struct {
    Name           string
    DisplayName    string
    Summarization  bool  // NEW — designates this model for summarization tasks
}
```

**`CreateProviderConfigParams.Models`** and **`UpdateProviderConfigParams.Models`** — the `ModelConfig` change flows through automatically since they use `[]ModelConfig`.

**OpenAPI schema** (`runtime/internal/agentapi/openapi.yaml`) — update `ModelConfig` schema:
```yaml
ModelConfig:
  type: object
  required: [name]
  properties:
    name:
      type: string
      minLength: 1
    displayName:
      type: string
    summarization:
      type: boolean
      description: >
        Designates this model for summarization tasks (e.g. generating session titles).
        If multiple models have this enabled, the first one found is used.
      default: false
```

**Database**: If `ModelConfig` is stored as a JSON field within the provider record (likely given GORM + the nested structure), no separate migration is needed — the new boolean field will have its zero value (`false`) for existing records. If models are in a separate table, an `ALTER TABLE ADD COLUMN summarization BOOLEAN DEFAULT FALSE` migration is needed.

**UI** (`apps/sonal-ui/src/pages/Providers.svelte`) — in the model configuration form section:
- Add a checkbox labeled **"Summarization"** for each model entry.
- Below the checkbox, a subtle hint (small muted text, non-disruptive): *"Use this model for summarization tasks (e.g. session titles). Prefer fast, inexpensive models."*
- The checkbox value maps to the `summarization` field in the API request.

### 4.5 File-Based Metadata Store (`runtime/internal/file_session_metadata_store.go` — new file)

- Stores a single JSON index file per app+user: `{baseDir}/{appName}/{userID}/_sessions_index.json`
- The index is a JSON array of `SessionMetadata` objects.
- **`Save` (upsert)**: read index → upsert entry by session ID (create if absent, update if present) → write index (atomic via temp file + rename). This upsert behavior ensures that if the initial create-time save failed, a subsequent update-time save will create the record, reducing chances of lost sessions.
- `List`: read index → sort by `UpdatedAt` desc → compute total count → apply offset/limit → return slice + total.
- `Delete`: read index → remove entry → write index.
- Thread-safe via `sync.RWMutex`.

### 4.6 Database-Backed Metadata Store (`runtime/internal/db_session_metadata_store.go` — new file)

- GORM model `sessionMetadataRecord` with columns: `session_id` (PK), `app_name`, `user_id`, `title`, `created_at`, `updated_at`.
- Table name respects the configurable table prefix (via `GormSonalmodTablesOpts`).
- **`Save` (upsert)**: GORM `Save` — creates or updates based on PK. Same upsert rationale as file store.
- `List`: `WHERE app_name = ? AND user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?` + separate `COUNT(*)` query for total.
- `Delete`: `DELETE WHERE session_id = ? AND app_name = ? AND user_id = ?`.
- Implements `AutoMigratable` so `AutoMigrateAll` picks it up.

### 4.7 Session Service Decorator (`runtime/internal/session_service_decorator.go` — new file)

Wraps `session.Service`, `SessionMetadataStore`, and `Summarizer`:

```go
type sessionServiceDecorator struct {
    inner         session.Service
    metadataStore SessionMetadataStore
    summarizer    Summarizer
    logger        *slog.Logger
}
```

- **`Create`**: delegates to `inner.Create`, then calls `metadataStore.Save` (upsert) with empty title and current timestamp as both `CreatedAt` and `UpdatedAt`. Metadata save errors are logged but do not fail the create (best-effort).
- **`AppendEvent`**: delegates to `inner.AppendEvent`. Then calls `metadataStore.Save` (upsert) to update `UpdatedAt`. If the event has a user-role content with text and the session's metadata title is currently empty:
  1. Call `summarizer.Summarize(ctx, messageText)` — the summarizer handles its own fallback internally, so this always returns a usable title.
  2. Set the title from the result.
  3. Metadata save errors are logged but do not fail the append (best-effort).
- **`Delete`**: delegates to `inner.Delete`, then calls `metadataStore.Delete`. Metadata delete errors are logged.
- **`Get`, `List`**: pure delegation to `inner`.
- The decorator implements `session.Service` so it can replace the inner service transparently.

### 4.8 Factory / Wiring Changes

**`session_service_factory.go`** — `NewSessionServiceFromConfig` already creates the `session.Service`. Changes:
- Add `SessionMetadataStore` and `Summarizer` to `SessionServiceFactoryDeps` (or create them internally based on storage type).
- After creating the base session service, wrap it with `sessionServiceDecorator`.
- Return the decorator as the `session.Service`.
- Also return the `SessionMetadataStore` (or make it accessible) so `AgentRunnerFactory` can use it for listing.

**`AgentRunnerFactory`** (`agentrun.go`) — add a `sessionMetadataStore` field and a `ListSessions` method:
```go
func (f *AgentRunnerFactory) ListSessions(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error) {
    return f.sessionMetadataStore.List(ctx, params)
}
```

### 4.9 Public Contract Changes (`runtime/agent/runner.go`)

Add new types and extend the interface:

```go
type SessionMetadata = internal.SessionMetadata
type ListSessionsParams = internal.ListSessionMetadataParams
type ListSessionsResult = internal.ListSessionMetadataResult

type AgentRunner interface {
    Run(ctx context.Context, params RunParams) (*RunResult, error)
    ReadSession(ctx context.Context, params ReadSessionParams) (*ReadSessionResult, error)
    ListSessions(ctx context.Context, params ListSessionsParams) (*ListSessionsResult, error)
}
```

`Runner.ListSessions` delegates to `runnerFactory.ListSessions` with `AppName` = `defaultRunnerAppName`.

### 4.10 BackgroundRunner Changes (`runtime/internal/background_runner.go`)

- `backgroundRunnerDep` interface gets `ListSessions`.
- `BackgroundRunner.ListSessions` delegates to `br.runner.ListSessions` (no background logic needed — it's a read operation).

### 4.11 HTTP Handler Wiring (`runtime/httpapi/handler.go`)

No changes needed beyond what oapi-codegen generates — `AgentAPIServer` already receives `agent.AgentRunner` which will include `ListSessions`.

### 4.12 OpenAPI Spec Changes — Session Listing (`runtime/internal/agentapi/openapi.yaml`)

New endpoint:

```yaml
/sessions:
  get:
    tags: [Sessions]
    operationId: listSessions
    summary: List sessions for the authenticated user
    description: |
      Returns session metadata (id, title, timestamps) for all sessions
      belonging to the authenticated user, sorted by last update time (most recent first).
    parameters:
      - name: limit
        in: query
        required: true
        description: Maximum number of sessions to return (1–100).
        schema:
          type: integer
          minimum: 1
          maximum: 100
      - name: offset
        in: query
        required: false
        description: Number of sessions to skip (for pagination). Defaults to 0.
        schema:
          type: integer
          minimum: 0
          default: 0
    responses:
      '200':
        description: Paginated list of session metadata.
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SessionListResponse'
      '400':
        $ref: '#/components/responses/BadRequest'
      '401':
        $ref: '#/components/responses/Unauthorized'
      '500':
        $ref: '#/components/responses/InternalError'
```

New schemas:

```yaml
SessionMetadata:
  type: object
  required: [sessionId, title, createdAt, updatedAt]
  properties:
    sessionId:
      type: string
    title:
      type: string
      description: >
        Session title. Always non-empty — resolved by the backend.
        Generated via LLM summarization (if a summarization model is designated),
        derived from the first user message, or a date-based fallback for sessions
        without a stored title.
    createdAt:
      type: string
      format: date-time
    updatedAt:
      type: string
      format: date-time
  additionalProperties: false

SessionListResponse:
  type: object
  required: [sessions, total]
  properties:
    sessions:
      type: array
      items:
        $ref: '#/components/schemas/SessionMetadata'
    total:
      type: integer
      description: Total number of sessions matching the query (before limit/offset).
  additionalProperties: false
```

After spec changes: `go generate ./internal/agentapi` to regenerate, then `make generate-api` in `apps/sonal-ui`.

### 4.13 API Server Handler (`runtime/internal/agentapi/server.go`)

New method on `AgentAPIServer`:

```go
func (s *AgentAPIServer) ListSessions(w http.ResponseWriter, r *http.Request, params ListSessionsParams) {
    ctx := r.Context()
    id := callerid.FromContext(ctx)
    if id == nil {
        writeProblemDetails(w, http.StatusUnauthorized, "Unauthorized", "authentication required")
        return
    }
    result, err := s.runner.ListSessions(ctx, agent.ListSessionsParams{
        AppName: "sonalmod-runtime",
        UserID:  id.UserID(),
        Limit:   params.Limit,
        Offset:  derefOrDefault(params.Offset, 0),
    })
    if err != nil {
        s.logger.ErrorContext(ctx, "ListSessions: runner", "err", err)
        writeProblemDetails(w, http.StatusInternalServerError, "Internal Server Error", "failed to list sessions")
        return
    }
    // Map internal SessionMetadata → API SessionMetadata
    // Apply title fallback: if metadata.Title == "", use "Session " + metadata.CreatedAt.Format("Jan 2 15:04")
    // This is a cheap string operation — NEVER triggers LLM
    // ... encode JSON response with sessions + total
}
```

**Title fallback resolution** happens in the mapper (at listing time, never at title-generation time): if `metadata.Title` is empty (e.g. legacy sessions or sessions with no messages yet), the mapper generates a fallback title: `"Session " + metadata.CreatedAt.Format("Jan 2 15:04")`. This is a cheap string operation — **listing never triggers LLM calls**.

Note: `AppName` is hardcoded to `"sonalmod-runtime"` matching `defaultRunnerAppName` in `runner.go`. This is consistent with how `ReadSession` works in the existing handler (the server knows the app name).

### 4.14 UI: API Client (`apps/sonal-ui/src/lib/agentapi/client.ts`)

Add to `SonalAgentApi` interface and implementation:

```typescript
listSessions(params: { limit: number; offset?: number }): Promise<SessionListResponse>
```

Implementation: `GET /sessions?limit={limit}&offset={offset}` via openapi-fetch, same pattern as `listProviders`.

New type export in `types.ts`:
```typescript
export type SessionMetadata = components['schemas']['SessionMetadata']
export type SessionListResponse = components['schemas']['SessionListResponse']
```

### 4.15 UI: Provider Config — Summarization Checkbox (`apps/sonal-ui/src/pages/Providers.svelte`)

In the model configuration section of the provider form:
- Add a checkbox for each model entry, bound to `model.summarization` (boolean).
- Label: **"Summarization"**
- Below checkbox, a hint in small, muted text (non-disruptive): *"Use this model for summarization tasks (e.g. session titles). Prefer fast, inexpensive models."*
- The checkbox value is sent in create/update provider API requests as part of the `models[]` array.

### 4.16 UI: Session List Component (`apps/sonal-ui/src/components/SessionList.svelte` — new file)

- Receives session list data and currently active session ID as props.
- Renders a scrollable list of session entries.
- Each entry shows: title and relative time (e.g. "2 hours ago").
- Active session entry is visually highlighted.
- Click navigates to `/chat/{sessionId}`.
- "New chat" button at the top (mirrors existing toolbar behavior).

### 4.17 UI: Chat Page Layout Changes (`apps/sonal-ui/src/pages/Chat.svelte`)

- Chat page layout changes from single-column to sidebar + main content.
- Sidebar contains `SessionList`.
- On mount (and after each send completes), fetch session list via `agentApi.listSessions({ limit: 50 })`.
- Pass current `params.sessionId` to `SessionList` for active highlighting.
- Remove the existing "New chat" button from the toolbar (moved to sidebar).
- Sidebar should be collapsible on narrow viewports (responsive).

### 4.18 UI: Wireframe Update (`apps/sonal-ui/ui-wireframe.md`)

Update to document:
- New sidebar layout on Chat page.
- Session list behavior, states (loading, empty, error, populated).
- Interaction between sidebar and chat area.

## 5. Key Architectural Decisions

1. **Separate metadata store vs. using ADK's `session.Service.List()`**: ADK's `List()` returns full `Session` objects including all events. Loading all events for every session just to show a list is wasteful. A separate lightweight metadata store (`SessionMetadataStore`) indexes only the fields needed for listing (ID, title, timestamps). This keeps listing O(1) per session instead of O(events).

2. **Decorator pattern for metadata sync**: A `sessionServiceDecorator` wraps `session.Service` to intercept `Create`, `AppendEvent`, and `Delete`. This ensures metadata stays in sync automatically without requiring callers to manage two services. The decorator is transparent — consumers still see a `session.Service`.

3. **`Summarizer` as a general-purpose abstraction**: Text summarization is behind a generic `Summarizer` interface — not tied to session titles. The truncating implementation is the default; the provider-aware implementation activates when a summarization model is designated in provider config. This keeps the decorator simple, makes the summarization strategy testable in isolation, and allows reuse for future summarization needs.

4. **Per-provider model checkbox for summarization (fully dynamic)**: The summarization model is designated via a `summarization` checkbox on the provider model config UI. This is fully dynamic — no static YAML config, no app restart needed. The `LLMSummarizer` checks provider config on every call. If multiple models are designated, the first one found is used. A non-disruptive hint guides users to prefer fast, inexpensive models.

5. **Summarizer handles its own fallback**: The `LLMSummarizer` internally falls back to `NewTruncatingSummarizer()` on any failure (no designated model, LLM error, provider error). Callers (the decorator) always receive a usable title from `Summarize()` — they never need fallback logic.

6. **Title obligatory at API level, cheap fallback at listing time**: The API schema requires `title` as a non-empty string. At listing time, if a session has no stored title, the mapper uses a cheap string-based fallback: `"Session Apr 16 14:30"`. **Listing never triggers LLM calls.** LLM-based title generation only happens once per session, at `AppendEvent` time (first user message).

7. **Upsert semantics for metadata save**: All metadata `Save` operations use upsert (create-if-absent, update-if-present). If the initial create-time metadata save failed (best-effort), the subsequent `AppendEvent`-time save will create the record instead of silently losing it. This reduces chances of orphaned sessions missing from the listing.

8. **Best-effort metadata operations**: Metadata save/update failures are logged but do not fail the primary operation (session create, event append). The metadata store is a secondary index — the ADK session storage remains the source of truth for session data. Listing may be temporarily stale, but core functionality is unaffected.

9. **Offset/limit pagination**: Uses `limit` (required, max 100, no default — caller must specify) and `offset` (optional, default 0). The response includes `total` count for the UI to render pagination controls or infinite scroll. Simple and sufficient for the expected session volume.

10. **`ListSessions` on `AgentRunner` interface**: Session listing is fundamental enough to belong on the main runner interface rather than a separate `SessionLister` interface. This mirrors how `ReadSession` is already on the interface.

11. **Sidebar on Chat page (not a separate route)**: Placing the session list in a sidebar provides quick session switching without full page navigation, consistent with modern chat application patterns.

## 6. Uncertainties

1. **ADK database session service `AppendEvent` signature**: The `AppendEvent` method takes a `session.Session` and `*session.Event`. In the decorator, we need to extract the user ID and session ID from the session to update metadata. We need to verify the session parameter carries these fields reliably across all backends.

2. **Title extraction timing**: The decorator intercepts `AppendEvent` to set the title from the first user message. However, ADK calls `AppendEvent` for every event type (model responses, tool calls, etc.), not just user messages. The decorator must check `event.Content.Role == "user"` and only extract title when the metadata title is still empty. Need to verify the role value ADK uses for user messages.

3. **Concurrency on file-based metadata index**: The file metadata store uses a single JSON index file per app+user. Concurrent writes need mutex protection. The existing `fileSessionService` uses `sync.RWMutex` — the metadata store should follow the same pattern.

4. **Responsive sidebar behavior**: The exact breakpoint and mechanism for collapsing the sidebar on narrow viewports needs design refinement during UI implementation.

5. **LLM title generation latency**: The `LLMSummarizer` makes a network call to an LLM provider. If this is synchronous in the decorator's `AppendEvent`, it adds latency to every first user message. Consider whether generation should be async (goroutine) with the title updating later, or if the latency is acceptable given it only happens once per session. Recommendation: start synchronous; optimize to async if latency is noticeable.

6. **ModelConfig storage format**: If `ModelConfig` is stored as a JSON column within the provider record (nested struct), the new `Summarization` field will default to `false` for existing records without migration. If models are in a separate table, an explicit DB migration may be needed. Verify storage format before implementing.

## 7. Related Files

### Existing files to modify

| File | Change |
|------|--------|
| `runtime/internal/providers_config.go` | Add `Summarization bool` to `ModelConfig` |
| `runtime/internal/agentrun.go` | Add `sessionMetadataStore` field to `AgentRunnerFactory`, add `ListSessions` method |
| `runtime/internal/session_service_factory.go` | Create metadata store + summarizer, wrap session service with decorator |
| `runtime/internal/background_runner.go` | Add `ListSessions` to `backgroundRunnerDep` and `BackgroundRunner` |
| `runtime/internal/automigrate.go` | Include DB metadata store in auto-migration |
| `runtime/internal/models_locator.go` | May need adjustment for summarizer's use of `ResolveModel` |
| `runtime/agent/runner.go` | Add `ListSessions` to `AgentRunner` interface and `Runner` struct, export new types |
| `runtime/httpapi/handler.go` | No direct changes (auto-wired via `agent.AgentRunner`) |
| `runtime/internal/agentapi/openapi.yaml` | Add `GET /sessions` endpoint, `SessionMetadata`/`SessionListResponse` schemas, update `ModelConfig` schema with `summarization` |
| `runtime/internal/agentapi/server.go` | Add `ListSessions` handler + response mapper with title fallback |
| `apps/sonalmod/internal/runtime.go` | Wire `LLMSummarizer` with provider service + models locator |
| `apps/sonal-ui/src/lib/agentapi/client.ts` | Add `listSessions()` method |
| `apps/sonal-ui/src/lib/agentapi/types.ts` | Export `SessionMetadata`, `SessionListResponse` |
| `apps/sonal-ui/src/pages/Providers.svelte` | Add summarization checkbox + hint to model config form |
| `apps/sonal-ui/src/pages/Chat.svelte` | Sidebar layout, session list integration |
| `apps/sonal-ui/ui-wireframe.md` | Document sidebar behavior |

### New files to create

| File | Purpose |
|------|---------|
| `runtime/internal/session_metadata.go` | `SessionMetadata` type, `SessionMetadataStore` interface, pagination types |
| `runtime/internal/file_session_metadata_store.go` | File-based metadata store implementation |
| `runtime/internal/file_session_metadata_store_test.go` | Tests |
| `runtime/internal/db_session_metadata_store.go` | Database-backed metadata store implementation |
| `runtime/internal/db_session_metadata_store_test.go` | Tests |
| `runtime/internal/summarizer.go` | `Summarizer` interface + `TruncatingSummarizer` |
| `runtime/internal/summarizer_test.go` | Tests |
| `runtime/internal/llm_summarizer.go` | `LLMSummarizer` — dynamic LLM with truncation fallback |
| `runtime/internal/llm_summarizer_test.go` | Tests |
| `runtime/internal/session_service_decorator.go` | Decorator wrapping `session.Service` + metadata sync + summarizer |
| `runtime/internal/session_service_decorator_test.go` | Tests |
| `runtime/internal/agentapi/session_handlers.go` | `ListSessions` handler (separate file following `provider_handlers.go` pattern) |
| `runtime/internal/agentapi/session_mapper.go` | Map internal `SessionMetadata` → API `SessionMetadata` with title fallback |
| `apps/sonal-ui/src/components/SessionList.svelte` | Session list sidebar component |
| `apps/sonal-ui/src/components/SessionList.test.ts` | Tests |

## 8. Task List

TDD approach is followed throughout. Each task leaves the codebase in a buildable, lint/test-passing state per module completion protocol.

---

**Task 1.1: Define `SessionMetadata` type and `SessionMetadataStore` interface**
- Create `runtime/internal/session_metadata.go`
- Add `SessionMetadata` struct: `SessionID`, `AppName`, `UserID`, `Title`, `CreatedAt`, `UpdatedAt`
- Add `ListSessionMetadataParams` struct: `AppName`, `UserID`, `Limit` (int), `Offset` (int)
- Add `ListSessionMetadataResult` struct: `Sessions []SessionMetadata`, `Total int`
- Add `SessionMetadataStore` interface: `Save` (upsert), `List` (returns `*ListSessionMetadataResult`), `Delete`
- Add doc comment on `Save` clarifying upsert semantics
- No tests needed (pure types/interface, no logic)
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Verify build passes
- Write summary to `docs/implementation/plan-session-listing/summary-task-1.1.md`
- All checks from completion protocol must be passed

---

**Task 1.2: Implement file-based `SessionMetadataStore`**
- Create `runtime/internal/file_session_metadata_store.go`
- Implement `fileSessionMetadataStore` struct with JSON index file storage
- Storage layout: `{baseDir}/{appName}/{userID}/_sessions_index.json`
- Thread-safe via `sync.RWMutex`
- `Save` (upsert): read index → upsert by session ID (create if absent, update if present) → write atomically (temp file + rename)
- `List`: read index → sort by `UpdatedAt` desc → compute total count → apply offset/limit → return slice + total
- `Delete`: read index → remove entry → write atomically
- Write failing tests first in `runtime/internal/file_session_metadata_store_test.go`:
  - Save creates new metadata entry
  - Save updates existing metadata entry (upsert)
  - Save creates entry even when it was never created before (upsert recovery)
  - List returns entries sorted by updatedAt desc
  - List returns empty slice when no sessions exist
  - List with offset skips entries, total reflects full count
  - List with limit caps results, total reflects full count
  - List with offset + limit works together correctly
  - Delete removes entry
  - Delete of non-existent session does not error
- Run affected tests: `go test -v ./internal/ --run TestFileSessionMetadataStore`
  - Verify failure is expectation-based (not compilation errors)
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-1.2.md`
- All checks from completion protocol must be passed

---

**Task 1.3: Implement database-backed `SessionMetadataStore`**
- Create `runtime/internal/db_session_metadata_store.go`
- GORM model `sessionMetadataRecord` with explicit column names: `session_id`, `app_name`, `user_id`, `title`, `created_at`, `updated_at`
- Respect `GormSonalmodTablesOpts` table prefix
- Implement `AutoMigratable` interface
- `Save` (upsert): GORM `Save` — creates or updates based on PK
- `List`: `WHERE app_name = ? AND user_id = ? ORDER BY updated_at DESC LIMIT ? OFFSET ?` + separate `COUNT(*)` query for total
- `Delete`: `DELETE WHERE ...`
- Write failing tests first in `runtime/internal/db_session_metadata_store_test.go`:
  - Save creates new metadata entry
  - Save updates existing metadata entry (upsert)
  - List returns entries sorted by updatedAt desc
  - List returns empty slice when no sessions
  - List with offset/limit returns correct slice and total
  - Delete removes entry
  - Delete of non-existent session does not error
- Run affected tests: `go test -v ./internal/ --run TestDbSessionMetadataStore`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Update `runtime/internal/automigrate.go` to include the DB metadata store
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-1.3.md`
- All checks from completion protocol must be passed

---

**Task 2.1: Implement `Summarizer` interface and truncating implementation**
- Create `runtime/internal/summarizer.go`
- Define `Summarizer` interface with `Summarize(ctx, text) (string, error)`
- Implement `TruncatingSummarizer`:
  - Truncates to 50 chars at a word boundary
  - Appends `"..."` if truncated
  - Returns original text if under 50 chars
  - Never returns error
- Write failing tests in `runtime/internal/summarizer_test.go`:
  - Short text returned as-is
  - Long text truncated at word boundary with `"..."`
  - Text exactly at 50 chars returned as-is
  - Empty text returns empty string
- Run affected tests: `go test -v ./internal/ --run TestTruncatingSummarizer`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-2.1.md`
- All checks from completion protocol must be passed

---

**Task 2.2: Implement `LLMSummarizer`**
- Create `runtime/internal/llm_summarizer.go`
- Implement `LLMSummarizer` struct:
  - Fields: `providerService ProvidersConfigService`, `modelsLocator *ModelsLocator`, `fallback Summarizer` (`*TruncatingSummarizer`), `logger *slog.Logger`
  - `Summarize(ctx, text)`:
    1. Scan providers for first model with `Summarization: true`
    2. If none found → delegate to `fallback.Summarize(ctx, text)`
    3. Resolve LLM via `modelsLocator.ResolveModel(ctx, "provider/model")`
    4. Call LLM with prompt: *"Generate a concise title (max 8 words, max 50 characters) for a conversation that starts with: \<text truncated to 200 bytes (same algorithm as TruncatingSummarizer)\>"*
    5. On any error → log, delegate to `fallback.Summarize(ctx, text)`
    6. Return generated title
- Write failing tests in `runtime/internal/llm_summarizer_test.go`:
  - No summarization model designated → falls back to truncation
  - Summarization model designated → calls LLM, returns title
  - LLM error → falls back to truncation, logs error
  - Provider list error → falls back to truncation, logs error
  - Multiple models with summarization → uses first one found
- Run affected tests: `go test -v ./internal/ --run TestLLMSummarizer`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-2.2.md`
- All checks from completion protocol must be passed

---

**Task 3.1: Add `Summarization` field to `ModelConfig`**
- Update `runtime/internal/providers_config.go`: add `Summarization bool` to `ModelConfig`
- Verify `CreateProviderConfigParams` and `UpdateProviderConfigParams` use `[]ModelConfig` — the new field flows through automatically
- Update `runtime/internal/agentapi/openapi.yaml`: add `summarization` boolean (default false) to `ModelConfig` schema
- Regenerate: `go generate ./internal/agentapi`
- Regenerate UI types: `make generate-api` from `apps/sonal-ui`
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Run affected lint/test: `npx nx test sonal-ui --skipNxCache && npx nx lint sonal-ui --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-3.1.md`
- All checks from completion protocol must be passed

---

**Task 4.1: Implement `sessionServiceDecorator`**
- Create `runtime/internal/session_service_decorator.go`
- Struct wraps `session.Service` + `SessionMetadataStore` + `Summarizer` + `*slog.Logger`
- Implement `session.Service` interface:
  - `Create`: delegate → save metadata via upsert (empty title, current timestamps). Log errors on metadata save.
  - `AppendEvent`: delegate → upsert metadata with updated `UpdatedAt`. If `event.Content != nil && event.Content.Role == "user"` and metadata title is empty, call `summarizer.Summarize(ctx, messageText)` and set title from result. Log errors on metadata save.
  - `Delete`: delegate → delete metadata. Log errors on metadata delete.
  - `Get`: pure delegation.
  - `List`: pure delegation.
- Write failing tests first in `runtime/internal/session_service_decorator_test.go`:
  - Create delegates and saves metadata via upsert
  - AppendEvent delegates and upserts metadata updatedAt
  - AppendEvent calls summarizer for first user message when title is empty
  - AppendEvent does not overwrite existing title
  - AppendEvent with non-user role does not set title
  - Delete delegates and deletes metadata
  - Metadata save failure does not fail the primary Create
  - Metadata save failure does not fail AppendEvent
  - AppendEvent upserts metadata even if Create-time save failed (recovery)
- Run affected tests: `go test -v ./internal/ --run TestSessionServiceDecorator`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-4.1.md`
- All checks from completion protocol must be passed

---

**Task 4.2: Wire decorator into session service factory**
- Update `runtime/internal/session_service_factory.go`:
  - Add `Summarizer` to `SessionServiceFactoryDeps`
  - After creating the base `session.Service`, create the appropriate `SessionMetadataStore` (file or DB based on same storage type selection)
  - Wrap with `sessionServiceDecorator` using the summarizer
  - Return both the decorated `session.Service` and the `SessionMetadataStore` (new return value or new field on a result struct)
- Update `NewSessionServiceFromConfig` signature/return to expose the metadata store
- Update callers (`runtime/agent/runner.go` `NewRunner`) to handle the metadata store
- Store the metadata store on `Runner` for use in `ListSessions`
- Write failing tests:
  - Factory with file storage creates decorator + file metadata store
  - Factory with database storage creates decorator + DB metadata store
  - Factory with memory storage creates decorator + in-memory/file metadata store (decide behavior)
- Run affected tests: `go test -v ./internal/ --run TestSessionServiceFactory`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-4.2.md`
- All checks from completion protocol must be passed

---

**Task 5.1: Add `ListSessions` to `AgentRunnerFactory`**
- Add `sessionMetadataStore SessionMetadataStore` field to `AgentRunnerFactory`
- Update `AgentRunnerFactoryDeps` to include `SessionMetadataStore`
- Add `ListSessions(ctx, ListSessionMetadataParams) (*ListSessionMetadataResult, error)` method
- Update `NewRunner` in `runtime/agent/runner.go` to pass metadata store to factory deps
- Write failing tests:
  - `AgentRunnerFactory.ListSessions` delegates to metadata store
- Run affected tests: `go test -v ./internal/ --run TestAgentRunnerFactory`
  - Verify failure is expectation-based
- Implement the logic
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-5.1.md`
- All checks from completion protocol must be passed

---

**Task 5.2: Add `ListSessions` to public `AgentRunner` interface and `Runner`**
- In `runtime/agent/runner.go`:
  - Export type aliases: `SessionMetadata = internal.SessionMetadata`, `ListSessionsParams = internal.ListSessionMetadataParams`, `ListSessionsResult = internal.ListSessionMetadataResult`
  - Add `ListSessions(ctx context.Context, params ListSessionsParams) (*ListSessionsResult, error)` to `AgentRunner` interface
  - Implement on `Runner`: delegate to `runnerFactory.ListSessions` with `AppName: defaultRunnerAppName`
- Update `var _ AgentRunner = (*Runner)(nil)` compile-time check (should still pass)
- Write failing tests:
  - `Runner.ListSessions` returns sessions from metadata store
- Run affected tests: `go test -v ./agent/ --run TestRunner`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-5.2.md`
- All checks from completion protocol must be passed

---

**Task 5.3: Add `ListSessions` to `BackgroundRunner`**
- In `runtime/internal/background_runner.go`:
  - Add `ListSessions` to `backgroundRunnerDep` interface
  - Implement `BackgroundRunner.ListSessions` — pure delegation to `br.runner.ListSessions`
- Update mock if auto-generated (re-run mockery if needed)
- Write failing tests:
  - `BackgroundRunner.ListSessions` delegates to underlying runner
- Run affected tests: `go test -v ./internal/ --run TestBackgroundRunner`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-5.3.md`
- All checks from completion protocol must be passed

---

**Task 6.1: Add `GET /sessions` to OpenAPI spec and regenerate**
- Edit `runtime/internal/agentapi/openapi.yaml`:
  - Add `Sessions` tag
  - Add `/sessions` path with `GET` → `listSessions` operation
  - Add `limit` (required, integer, min 1, max 100, **no default**) and `offset` (optional, integer, min 0, default 0) query parameters
  - Add `SessionMetadata` schema (sessionId, title, createdAt, updatedAt — all required)
  - Add `SessionListResponse` schema (sessions array + total integer — both required)
- Regenerate: `go generate ./internal/agentapi`
- Regenerate UI types: `make generate-api` from `apps/sonal-ui`
- Verify both build: `npx nx lint runtime --skipNxCache` and `npx nx lint sonal-ui --skipNxCache`
- No new tests needed (codegen verification only)
- Write summary to `docs/implementation/plan-session-listing/summary-task-6.1.md`
- All checks from completion protocol must be passed

---

**Task 6.2: Implement `ListSessions` handler in `AgentAPIServer`**
- Create `runtime/internal/agentapi/session_handlers.go`
- Create `runtime/internal/agentapi/session_mapper.go` — map `internal.SessionMetadata` → API `SessionMetadata`:
  - If `metadata.Title == ""`, apply cheap string-based fallback: `"Session " + metadata.CreatedAt.Format("Jan 2 15:04")`
  - **Listing never triggers LLM calls** — this is purely string formatting
  - This ensures the API always returns a non-empty title
- Implement `ListSessions(w, r, params)` on `AgentAPIServer`:
  - Extract caller identity from context
  - Parse `limit` (required, no default) and `offset` (optional, default 0) from generated params
  - Call `s.runner.ListSessions(ctx, ...)`
  - Map results → `SessionListResponse` (with title fallback)
  - Encode JSON response
- Write failing tests:
  - Authenticated request returns session list with pagination
  - Unauthenticated request returns 401
  - Empty session list returns `{"sessions": [], "total": 0}`
  - Sessions with empty titles get fallback title in response
  - Internal error returns 500 with problem details
- Run affected tests: `go test -v ./internal/agentapi/ --run TestAgentAPIServer`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test runtime --skipNxCache && npx nx lint runtime --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-6.2.md`
- All checks from completion protocol must be passed

---

**Task 7.1: Add `listSessions` to UI API client**
- In `apps/sonal-ui/src/lib/agentapi/types.ts`: export `SessionMetadata` and `SessionListResponse`
- In `apps/sonal-ui/src/lib/agentapi/client.ts`:
  - Add `listSessions(params: { limit: number; offset?: number }): Promise<SessionListResponse>` to `SonalAgentApi` interface
  - Implement: `GET /sessions?limit={limit}&offset={offset}` via openapi-fetch (same pattern as `listProviders`)
- Write failing tests in `apps/sonal-ui/src/lib/agentapi/client.test.ts`:
  - `listSessions` returns session list on success
  - `listSessions` throws on API error
- Run affected tests: `npm run test:run -- --reporter=verbose`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test sonal-ui --skipNxCache && npx nx lint sonal-ui --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-7.1.md`
- All checks from completion protocol must be passed

---

**Task 7.2: Add summarization checkbox to provider config UI**
- Update `apps/sonal-ui/src/pages/Providers.svelte`:
  - In the model configuration section of the provider form, add a checkbox for each model entry bound to `model.summarization`
  - Label: **"Summarization"**
  - Below checkbox, subtle hint in small muted text: *"Use this model for summarization tasks (e.g. session titles). Prefer fast, inexpensive models."*
  - The checkbox value is included in create/update provider API requests as part of `models[]`
- Update `apps/sonal-ui/src/pages/Providers.test.ts`:
  - Summarization checkbox renders for each model
  - Checkbox value is sent in create/update requests
- Run affected tests: `npm run test:run -- --reporter=verbose`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test sonal-ui --skipNxCache && npx nx lint sonal-ui --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-7.2.md`
- All checks from completion protocol must be passed

---

**Task 7.3: Create `SessionList` component**
- Create `apps/sonal-ui/src/components/SessionList.svelte`
- Props: `sessions: SessionMetadata[]`, `activeSessionId: string | null`, `onNewChat: () => void`
- Renders:
  - "New chat" button at top
  - Scrollable list of session entries
  - Each entry: title, relative time string
  - Active session highlighted with distinct background
  - Click navigates to `/chat/{sessionId}`
- Follow DESIGN.md conventions (terminal-native, monospace, warm palette)
- Write tests in `apps/sonal-ui/src/components/SessionList.test.ts`:
  - Renders session entries with titles
  - Highlights active session
  - Calls onNewChat when "New chat" clicked
- Run affected tests: `npm run test:run -- --reporter=verbose`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test sonal-ui --skipNxCache && npx nx lint sonal-ui --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-7.3.md`
- All checks from completion protocol must be passed

---

**Task 7.4: Integrate session list sidebar into Chat page**
- Update `apps/sonal-ui/src/pages/Chat.svelte`:
  - Change layout from single column to sidebar (session list) + main content (chat)
  - Add `sessionList` state: `$state<SessionMetadata[]>([])`
  - Fetch sessions via `agentApi.listSessions({ limit: 50 })` on mount and after each send completes (`done` event)
  - Pass `sessionList`, `params.sessionId`, and `handleNewChat` to `SessionList` component
  - Remove existing "New chat" button from toolbar (moved to sidebar)
  - Sidebar width ~260px, collapsible on narrow viewports
- Update `apps/sonal-ui/ui-wireframe.md` to document new layout and sidebar behavior
- Write tests in `apps/sonal-ui/src/pages/Chat.test.ts`:
  - Session list is rendered in sidebar
  - Session list refreshes after send completes
  - (Additional integration-level tests as appropriate)
- Run affected tests: `npm run test:run -- --reporter=verbose`
  - Verify failure is expectation-based
- Implement
- Run affected tests, verify all pass
- Run affected lint/test: `npx nx test sonal-ui --skipNxCache && npx nx lint sonal-ui --skipNxCache`
- Write summary to `docs/implementation/plan-session-listing/summary-task-7.4.md`
- All checks from completion protocol must be passed

---

**Task 8.1: Wire `LLMSummarizer` in app and end-to-end verification**
- Update `apps/sonalmod/internal/runtime.go`:
  - Create `LLMSummarizer` with `ProvidersConfigService` + `ModelsLocator` + `NewTruncatingSummarizer()` as fallback
  - Pass `Summarizer` to session service factory
- Run full `make affected-lint-test` from repo root
- Verify all modules pass
- Update `runtime/AGENTS.md` public contract section to document `ListSessions` on `AgentRunner`, `SessionMetadata` type, and `Summarizer` interface
- Update `runtime/AGENTS.md` API Layer section to mention `GET /sessions` endpoint with pagination and `summarization` field on `ModelConfig`
- Update `apps/sonal-ui/AGENTS.md` if any conventions or architecture changes warrant it
- Write summary to `docs/implementation/plan-session-listing/summary-task-8.1.md`
- All checks from completion protocol must be passed

---

**Compress implementation summaries**

- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
