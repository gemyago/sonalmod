# Plan: Unified Sessions Storage

## 1. Introduction / Overview

**Problem:** Session storage is currently split into two separate, independently initialized components:
- `session.Service` (ADK interface) — stores full session data (events, state, identifiers)
- `SessionMetadataStore` (internal interface) — stores lightweight listing metadata (id, title, timestamps)

Both components share the same configuration (same `baseDir` for file storage, same `dsn` for database storage), yet they are constructed separately in the factory, threaded individually through `SessionServiceFromConfigResult`, passed as separate fields into `AgentRunnerFactory`, stored as separate fields on `AgentRunner` and `Runner`, and handled separately in `AutoMigrateAll`.

**Goal:** Introduce a `SessionsStorage` interface that unifies both components into a single composable unit — `FileSessionsStorage`, `DatabaseSessionsStorage`, and `MemorySessionsStorage`. Every consumer that today holds two session-related fields (`sessionService + sessionMetadataStore`) becomes a consumer of a single `SessionsStorage`.

---

## 2. Business Logic

- `SessionsStorage` is the single concept for "session persistence". It packages session data and metadata together because they always refer to the same physical store (same directory or same database).
- Session metadata exists separately from session data for performance reasons (cheap listing with pagination and titles vs. expensive full-session reads). This design is preserved — the two implementations remain separate internally but are exposed as one unit.
- The decorator (`sessionServiceDecorator`) keeps metadata in sync on every write to the ADK session service. This behavior is preserved: the decorator is updated to accept and implement `SessionsStorage` directly — no separate wrapper type needed.
- `AutoMigrate` is a capability of `SessionsStorage`. Database-backed implementations run both ADK schema migration and GORM metadata schema migration in one `AutoMigrate()` call. File and memory implementations are no-ops.

---

## 3. High-Level Architecture

```
SessionsStorage (interface, runtime/internal)
├── embeds session.Service    ← all ADK methods: Create, Get, List, Delete, AppendEvent
├── SaveMetadata(...)         ← distinct name (avoids conflict with session.Service.Delete/List)
├── ListMetadata(...)
├── DeleteMetadata(...)
└── AutoMigrate() error       ← schema migration (no-op for file/memory)

Concrete implementations (return structs, accept interfaces):
  *FileSessionsStorage        → embeds *fileSessionService (promotes session.Service methods)
                                 delegates SaveMetadata/ListMetadata/DeleteMetadata to *fileSessionMetadataStore
  *DatabaseSessionsStorage    → embeds session.Service (interface field, promotes ADK methods)
                                 delegates SaveMetadata/ListMetadata/DeleteMetadata to *DatabaseSessionMetadataStore
  *MemorySessionsStorage      → embeds session.Service (InMemoryService result)
                                 delegates to *memorySessionMetadataStore

Decorator:
  *sessionServiceDecorator    → accepts inner SessionsStorage ("accept interface")
                                 implements SessionsStorage ("return struct" from constructor)
                                 intercepts Create/AppendEvent/Delete to call inner.SaveMetadata/DeleteMetadata
                                 all other methods delegate to inner
                                 no separate metadataStore field — uses inner.SaveMetadata/ListMetadata/DeleteMetadata

Factory (NewSessionsStorage):
  picks raw impl by config → wraps with *sessionServiceDecorator → returns SessionsStorage
  (justified interface-return exception: dynamic factory, caller holds SessionsStorage)

Consumers (accept SessionsStorage interface):
  AgentRunnerFactory    → holds SessionsStorage (was: sessionService + sessionMetadataStore)
  AgentRunner           → holds SessionsStorage
  Runner (agent pkg)    → holds SessionsStorage (was: sessionSvc + adkSessionSvc + sessionMetadataStore)
```

---

## 4. Detailed Architecture

### 4.1 `SessionsStorage` Interface (new file: `runtime/internal/sessions_storage.go`)

`SessionsStorage` is in `runtime/internal` — the same package as all implementations, keeping ADK details fully internal per the module's architectural rule.

`SessionsStorage` embeds `session.Service` directly to extend the ADK contract rather than hide it. The ADK session methods (`Create`, `Get`, `List`, `Delete`, `AppendEvent`) are available directly on `SessionsStorage`. The metadata methods use distinct names (`SaveMetadata`, `ListMetadata`, `DeleteMetadata`) to avoid naming conflicts with `session.Service.List` and `session.Service.Delete`.

```go
// SessionsStorage is the unified session persistence component.
// It extends the ADK session service with session metadata operations and database migration.
// All implementations are in the same package; this interface is the only type consumers depend on.
type SessionsStorage interface {
    session.Service // embed: Create, Get, List, Delete, AppendEvent

    // SaveMetadata persists lightweight session metadata (upsert semantics).
    SaveMetadata(ctx context.Context, metadata SessionMetadata) error

    // ListMetadata returns a paginated list of session metadata.
    ListMetadata(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)

    // DeleteMetadata removes session metadata for the given session.
    DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error

    // AutoMigrate runs database schema migrations. No-op for file and in-memory backends.
    AutoMigrate() error
}
```

No tests needed for this type alone (it is an interface).

### 4.2 `FileSessionsStorage` (new file: `runtime/internal/file_sessions_storage.go`)

`*fileSessionService` is embedded by pointer — its methods (`Create`, `Get`, `List`, `Delete`, `AppendEvent`) are promoted, making `*FileSessionsStorage` satisfy `session.Service` automatically. The metadata methods delegate to an embedded `*fileSessionMetadataStore`.

```go
type FileSessionsStorage struct {
    *fileSessionService              // promoted session.Service methods
    meta *fileSessionMetadataStore
}

// NewFileSessionsStorage returns concrete *FileSessionsStorage (accept interface, return struct).
func NewFileSessionsStorage(baseDir string, logger *slog.Logger) (*FileSessionsStorage, error)

func (s *FileSessionsStorage) SaveMetadata(ctx context.Context, m SessionMetadata) error {
    return s.meta.Save(ctx, m)
}
func (s *FileSessionsStorage) ListMetadata(ctx context.Context, p ListSessionMetadataParams) (*ListSessionMetadataResult, error) {
    return s.meta.List(ctx, p)
}
func (s *FileSessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
    return s.meta.Delete(ctx, appName, userID, sessionID)
}
func (s *FileSessionsStorage) AutoMigrate() error { return nil }
```

Constructor calls `NewFileSessionService` and `NewFileSessionMetadataStore` with the same `baseDir`. Returns error if either fails.

Compile-time check: `var _ SessionsStorage = (*FileSessionsStorage)(nil)`

**Tests** (`file_sessions_storage_test.go`):
- `NewFileSessionsStorage` with valid dir → non-nil, no error
- `NewFileSessionsStorage` with empty dir → error
- `AutoMigrate()` returns nil
- Embedded `*fileSessionService` is reachable (type assertion confirms inner ADK service)
- `meta` field is `*fileSessionMetadataStore` (type assertion)

### 4.3 `DatabaseSessionsStorage` (new file: `runtime/internal/db_sessions_storage.go`)

The ADK DB session service is returned as `session.Service` from `NewDatabaseSessionService` (no concrete struct to embed by pointer). Embedding the `session.Service` interface as an anonymous field promotes all its methods to `*DatabaseSessionsStorage`.

```go
type DatabaseSessionsStorage struct {
    session.Service                  // embedded interface field — promotes ADK methods
    meta *DatabaseSessionMetadataStore
}

// NewDatabaseSessionsStorage returns concrete *DatabaseSessionsStorage.
func NewDatabaseSessionsStorage(dsn string, opts GormSonalmodTablesOpts) (*DatabaseSessionsStorage, error)

func (s *DatabaseSessionsStorage) SaveMetadata(ctx context.Context, m SessionMetadata) error {
    return s.meta.Save(ctx, m)
}
func (s *DatabaseSessionsStorage) ListMetadata(ctx context.Context, p ListSessionMetadataParams) (*ListSessionMetadataResult, error) {
    return s.meta.List(ctx, p)
}
func (s *DatabaseSessionsStorage) DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error {
    return s.meta.Delete(ctx, appName, userID, sessionID)
}
func (s *DatabaseSessionsStorage) AutoMigrate() error {
    if err := database.AutoMigrate(s.Service); err != nil {
        return fmt.Errorf("adk session migrate: %w", err)
    }
    return s.meta.AutoMigrate()
}
```

Constructor calls `NewDatabaseSessionService` and `NewDatabaseSessionMetadataStore` with the same `dsn` + opts. Sets `Service` field to the returned ADK service.

Compile-time checks: `var _ SessionsStorage = (*DatabaseSessionsStorage)(nil)` and `var _ AutoMigratable = (*DatabaseSessionsStorage)(nil)`

**Tests** (`db_sessions_storage_test.go`):
- `NewDatabaseSessionsStorage` with `":memory:"` DSN → non-nil, no error
- `NewDatabaseSessionsStorage` with empty DSN → error
- `AutoMigrate()` with `":memory:"` DSN → no error
- After `AutoMigrate()`, `Create(...)` succeeds

### 4.4 `MemorySessionsStorage` (new file: `runtime/internal/memory_sessions_storage.go`)

Follows the same pattern as `DatabaseSessionsStorage`: embeds `session.Service` as an anonymous interface field.

```go
type MemorySessionsStorage struct {
    session.Service                  // holds session.InMemoryService()
    meta *memorySessionMetadataStore
}

func NewMemorySessionsStorage() *MemorySessionsStorage

func (s *MemorySessionsStorage) SaveMetadata(...)   // delegates to s.meta.Save
func (s *MemorySessionsStorage) ListMetadata(...)   // delegates to s.meta.List
func (s *MemorySessionsStorage) DeleteMetadata(...) // delegates to s.meta.Delete
func (s *MemorySessionsStorage) AutoMigrate() error { return nil }
```

Compile-time check: `var _ SessionsStorage = (*MemorySessionsStorage)(nil)`

**Tests** (`memory_sessions_storage_test.go`):
- `NewMemorySessionsStorage()` → non-nil
- `AutoMigrate()` returns nil

### 4.5 Decorator Update (`runtime/internal/session_service_decorator.go`)

No new wrapper type needed. The existing `sessionServiceDecorator` is updated to accept `inner SessionsStorage` instead of `inner session.Service` + separate `metadataStore`. This eliminates the `metadataStore` field entirely: the decorator uses `inner.SaveMetadata/ListMetadata/DeleteMetadata` for all metadata operations.

```go
type sessionServiceDecorator struct {
    inner      SessionsStorage  // was: inner session.Service + separate metadataStore
    summarizer Summarizer
    logger     *slog.Logger
}

// NewSessionServiceDecorator accepts interface, returns concrete struct ("accept interface, return struct").
// The nolint:ireturn annotation is removed — this now returns the concrete type.
func NewSessionServiceDecorator(
    inner SessionsStorage,
    summarizer Summarizer,
    logger *slog.Logger,
) *sessionServiceDecorator
```

The intercepted ADK methods (`Create`, `AppendEvent`, `Delete`) call `d.inner.Create/AppendEvent/Delete` then use `d.inner.SaveMetadata/DeleteMetadata`. The `findSessionMetadata` helper uses `d.inner.ListMetadata`.

Pass-through ADK methods (`Get`, `List`) delegate to `d.inner.Get/List`.

The metadata methods on the decorator are simple pass-throughs:
```go
func (d *sessionServiceDecorator) SaveMetadata(ctx context.Context, m SessionMetadata) error {
    return d.inner.SaveMetadata(ctx, m)
}
// same for ListMetadata, DeleteMetadata
```

`AutoMigrate()` delegates to `d.inner.AutoMigrate()`.

Compile-time check: `var _ SessionsStorage = (*sessionServiceDecorator)(nil)`

**Tests**: existing decorator tests are updated to construct via `NewSessionServiceDecorator(inner SessionsStorage, ...)`. No separate `decoratedSessionsStorage` tests needed.

### 4.6 Factory Refactor (`runtime/internal/session_service_factory.go`)

```go
// NewSessionsStorage replaces NewSessionServiceFromConfig.
// Returns SessionsStorage (interface) — justified exception to "return struct":
// the concrete type varies by config and is always a *sessionServiceDecorator which is unexported.
// Callers depend only on the SessionsStorage contract.
func NewSessionsStorage(deps SessionServiceFactoryDeps) (SessionsStorage, error)
```

Internally: switch on `deps` flags/type → create raw concrete storage → wrap with `NewSessionServiceDecorator(raw, deps.Summarizer, deps.RootLogger)` → return.

`SessionServiceFromConfigResult`, `NewSessionServiceFromConfig`, and `newDecoratedSessionService` were removed in Task 5.1 once all callers used `NewSessionsStorage` only.

**Tests** (`session_service_factory_test.go`): update existing tests to use `NewSessionsStorage`. Assert `storage.SaveMetadata/ListMetadata/AutoMigrate` instead of accessing `res.ADKSessionService`.

### 4.7 `AgentRunnerFactory` and `AgentRunner` (`runtime/internal/agentrun.go`)

```go
// Before:
type AgentRunnerFactoryDeps struct {
    SessionService       session.Service
    SessionMetadataStore SessionMetadataStore
    ...
}

// After (accepts interface):
type AgentRunnerFactoryDeps struct {
    SessionStorage SessionsStorage
    ...
}
```

`AgentRunnerFactory` holds `sessionStorage SessionsStorage`. `AgentRunner` holds `sessionStorage SessionsStorage`.

Internal usages:
- `f.sessionService` → `f.sessionStorage` (it IS a `session.Service` — `SessionsStorage` embeds it)
- `a.sessionService` → `a.sessionStorage`
- `f.sessionMetadataStore.List(...)` → `f.sessionStorage.ListMetadata(...)`
- `a.sessionMetadataStore` → `a.sessionStorage`
- Pass to ADK runner: `runner.Config{SessionService: f.sessionStorage}` — works because `SessionsStorage` embeds `session.Service`

### 4.8 `Runner` Update (`runtime/agent/runner.go`)

```go
type Runner struct {
    sessionsStorage internal.SessionsStorage  // replaces: sessionSvc + adkSessionSvc + sessionMetadataStore
    ...
}

func (r *Runner) AutoMigrate() error {
    return r.sessionsStorage.AutoMigrate()
}
```

`NewRunner` calls `internal.NewSessionsStorage(...)` and passes `SessionStorage: ss` to `AgentRunnerFactoryDeps`.

### 4.9 `AutoMigrateAll` Simplification (`runtime/internal/automigrate.go`)

`Runner.AutoMigrate()` now calls `r.sessionsStorage.AutoMigrate()` directly — no longer calls `AutoMigrateAll` for session storage. If `AutoMigrateAll` has no remaining callers, delete it. If kept for other services, simplify its signature to `AutoMigrateAll(services ...AutoMigratable) error` (remove the special-case `session.Service` first argument).

---

## 5. Key Architectural Decisions

1. **`SessionsStorage` embeds `session.Service`, metadata methods use distinct names.** Directly embedding the ADK interface extends the contract rather than hiding it behind an accessor. Naming conflicts (`session.Service.List`/`Delete` vs `SessionMetadataStore.List`/`Delete`) are resolved by using `ListMetadata`/`DeleteMetadata`/`SaveMetadata` names. This avoids accessor-method indirection (`Session()`, `Metadata()`) and makes `SessionsStorage` directly usable as a `session.Service` where needed (e.g. passing to ADK `runner.Config.SessionService`).

2. **No `decoratedSessionsStorage` wrapper type.** The original plan introduced a separate `decoratedSessionsStorage` to make the decorator return `SessionsStorage`. This is unnecessary: `sessionServiceDecorator` itself implements `SessionsStorage` directly. Removing the wrapper eliminates an indirection layer.

3. **"Accept interface, return struct" applied throughout.** All concrete constructors return their concrete types: `NewFileSessionsStorage` → `*FileSessionsStorage`, `NewDatabaseSessionsStorage` → `*DatabaseSessionsStorage`, `NewMemorySessionsStorage` → `*MemorySessionsStorage`, `NewSessionServiceDecorator` → `*sessionServiceDecorator`. The only justified exception is the dynamic factory `NewSessionsStorage`, which returns `SessionsStorage` because the concrete type (`*sessionServiceDecorator`) is unexported and the selection is config-driven at runtime. All consumers hold `SessionsStorage` interface ("accept interface").

4. **`sessionServiceDecorator` loses its `metadataStore` field.** By accepting `inner SessionsStorage`, the decorator accesses metadata via `inner.SaveMetadata/ListMetadata/DeleteMetadata`. This removes redundant field wiring in both the decorator struct and its constructor — the inner storage is the single source of truth for all persistence.

5. **`SessionsStorage` and all implementations live in `runtime/internal`.** This keeps ADK details fully internal per the module rule. The `agent` package continues to expose `SessionMetadata`, `ListSessionsParams`, `ListSessionsResult` as type aliases of internal types — no change.

6. **No public API changes.** `WithFileSystemStorage`, `WithDatabaseStorage`, `AutoMigrate`, `ListSessions`, `SessionMetadata` etc. remain unchanged on `agent.Runner`.

---

## 6. Uncertainties

- **`AutoMigrateAll` callers outside `Runner.AutoMigrate`.** A search of the codebase showed only `Runner.AutoMigrate()` calls `AutoMigrateAll`. If no other callers remain after the refactor, delete `automigrate.go` entirely. If `DatabaseProvidersConfigService` or other services are expected to flow through this path in the future, simplify the signature and keep it.

---

## 7. Related Files

| File | Action |
|------|--------|
| `runtime/internal/sessions_storage.go` | **Create** — `SessionsStorage` interface |
| `runtime/internal/file_sessions_storage.go` | **Create** — `FileSessionsStorage` |
| `runtime/internal/file_sessions_storage_test.go` | **Create** |
| `runtime/internal/db_sessions_storage.go` | **Create** — `DatabaseSessionsStorage` |
| `runtime/internal/db_sessions_storage_test.go` | **Create** |
| `runtime/internal/memory_sessions_storage.go` | **Create** — `MemorySessionsStorage` |
| `runtime/internal/memory_sessions_storage_test.go` | **Create** |
| `runtime/internal/session_service_decorator.go` | **Modify** — decorator accepts `SessionsStorage`, implements `SessionsStorage`, loses `metadataStore` field |
| `runtime/internal/session_service_decorator_test.go` | **Modify** — update to new constructor signature |
| `runtime/internal/session_service_factory.go` | **Modify** — add `NewSessionsStorage`, remove `SessionServiceFromConfigResult` + old functions |
| `runtime/internal/session_service_factory_test.go` | **Modify** — update to `NewSessionsStorage` |
| `runtime/internal/agentrun.go` | **Modify** — `AgentRunnerFactoryDeps` + `AgentRunner` use `SessionsStorage` |
| `runtime/internal/automigrate.go` | **Modify or delete** — simplify/remove `AutoMigrateAll` |
| `runtime/agent/runner.go` | **Modify** — `Runner` holds `sessionsStorage`, `AutoMigrate` simplified |
| `runtime/internal/file_session_service.go` | No change |
| `runtime/internal/file_session_metadata_store.go` | No change |
| `runtime/internal/db_session_service.go` | No change |
| `runtime/internal/db_session_metadata_store.go` | No change |
| `runtime/internal/memory_session_metadata_store.go` | No change |

---

## 8. Task List

> Follow TDD: write failing tests first, verify failure is expected (not a compile error — add stubs), implement, verify green. Each task must leave `npx nx test runtime --skipNxCache` passing before moving to the next.

---

**Task 1.1: Define `SessionsStorage` interface**
- Create `runtime/internal/sessions_storage.go`
- Define `SessionsStorage` interface:
  - Embed `session.Service`
  - `SaveMetadata(ctx context.Context, metadata SessionMetadata) error`
  - `ListMetadata(ctx context.Context, params ListSessionMetadataParams) (*ListSessionMetadataResult, error)`
  - `DeleteMetadata(ctx context.Context, appName, userID, sessionID string) error`
  - `AutoMigrate() error`
- No tests required (interface only)
- Run `npx nx test runtime --skipNxCache`
  - Verify: no errors, existing tests still pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-1.1.md`
- Success criteria: as per module task completion protocol

---

**Task 1.2: Implement `FileSessionsStorage`**
- Create `runtime/internal/file_sessions_storage_test.go`
- Write failing tests:
  - `NewFileSessionsStorage` with valid dir → non-nil, no error
  - `NewFileSessionsStorage` with empty dir → error
  - `AutoMigrate()` returns nil
  - Inner `*fileSessionService` is accessible (embedded type assertion)
  - `meta` is `*fileSessionMetadataStore` (type assertion)
- Run `npx nx test runtime --skipNxCache` — compile errors → add stubs to make it compile
  - Compilation errors are **not acceptable** — add stubs, retry
- Create `runtime/internal/file_sessions_storage.go`:
  - `FileSessionsStorage` struct: `*fileSessionService` (embedded), `meta *fileSessionMetadataStore`
  - `NewFileSessionsStorage(baseDir string, logger *slog.Logger) (*FileSessionsStorage, error)` — calls `NewFileSessionService` + `NewFileSessionMetadataStore`, same `baseDir`
  - `SaveMetadata/ListMetadata/DeleteMetadata` delegate to `s.meta`
  - `AutoMigrate() error` returns nil
  - `var _ SessionsStorage = (*FileSessionsStorage)(nil)` compile check
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-1.2.md`
- Success criteria: as per module task completion protocol

---

**Task 1.3: Implement `DatabaseSessionsStorage`**
- Create `runtime/internal/db_sessions_storage_test.go`
- Write failing tests:
  - `NewDatabaseSessionsStorage` with `":memory:"` DSN → non-nil, no error
  - `NewDatabaseSessionsStorage` with empty DSN → error
  - `AutoMigrate()` with `":memory:"` DSN → no error
  - After `AutoMigrate()`, `Create(...)` succeeds
- Run `npx nx test runtime --skipNxCache` — compile errors → add stubs
- Create `runtime/internal/db_sessions_storage.go`:
  - `DatabaseSessionsStorage` struct: `session.Service` (embedded anonymous field), `meta *DatabaseSessionMetadataStore`
  - `NewDatabaseSessionsStorage(dsn string, opts GormSonalmodTablesOpts) (*DatabaseSessionsStorage, error)` — calls `NewDatabaseSessionService` + `NewDatabaseSessionMetadataStore`; sets `Service` field
  - `SaveMetadata/ListMetadata/DeleteMetadata` delegate to `s.meta`
  - `AutoMigrate()`: calls `database.AutoMigrate(s.Service)` then `s.meta.AutoMigrate()`, returns joined errors
  - `var _ SessionsStorage = (*DatabaseSessionsStorage)(nil)` compile check
  - `var _ AutoMigratable = (*DatabaseSessionsStorage)(nil)` compile check
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-1.3.md`
- Success criteria: as per module task completion protocol

---

**Task 1.4: Implement `MemorySessionsStorage`**
- Create `runtime/internal/memory_sessions_storage_test.go`
- Write failing tests:
  - `NewMemorySessionsStorage()` → non-nil
  - `AutoMigrate()` returns nil
- Run `npx nx test runtime --skipNxCache` — compile errors → add stubs
- Create `runtime/internal/memory_sessions_storage.go`:
  - `MemorySessionsStorage` struct: `session.Service` (embedded), `meta *memorySessionMetadataStore`
  - `NewMemorySessionsStorage() *MemorySessionsStorage` — calls `session.InMemoryService()` + `NewMemorySessionMetadataStore()`
  - `SaveMetadata/ListMetadata/DeleteMetadata` delegate to `s.meta`
  - `AutoMigrate() error` returns nil
  - `var _ SessionsStorage = (*MemorySessionsStorage)(nil)` compile check
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-1.4.md`
- Success criteria: as per module task completion protocol

---

**Task 2.1: Update `sessionServiceDecorator` to accept and implement `SessionsStorage`**
- Open `runtime/internal/session_service_decorator_test.go`
- Update existing test constructions: `NewSessionServiceDecorator` now takes `inner SessionsStorage` (not separate `session.Service` + `SessionMetadataStore`)
  - Use `NewMemorySessionsStorage()` as the inner storage in tests
- Add failing tests for the new behavior:
  - `NewSessionServiceDecorator` returns `*sessionServiceDecorator`
  - `var _ SessionsStorage = (*sessionServiceDecorator)(nil)` (compile check)
  - `SaveMetadata/ListMetadata/DeleteMetadata` delegate to `inner`
  - `AutoMigrate()` delegates to `inner`
  - On `Create` via the decorator, `ListMetadata` on the same decorator shows the metadata record (sync verified end-to-end)
- Run `npx nx test runtime --skipNxCache` — compile errors → add stubs
- Edit `runtime/internal/session_service_decorator.go`:
  - Change `inner session.Service` → `inner SessionsStorage`; remove `metadataStore SessionMetadataStore` field entirely
  - Update constructor: `NewSessionServiceDecorator(inner SessionsStorage, summarizer Summarizer, logger *slog.Logger) *sessionServiceDecorator` (returns concrete struct, remove `//nolint:ireturn`)
  - Intercepted ADK methods: replace `d.metadataStore.Save/Delete/List` with `d.inner.SaveMetadata/DeleteMetadata/ListMetadata`
  - `findSessionMetadata`: use `d.inner.ListMetadata`
  - Add `SaveMetadata/ListMetadata/DeleteMetadata` as pass-throughs to `d.inner`
  - Add `AutoMigrate()` that delegates to `d.inner.AutoMigrate()`
  - `var _ SessionsStorage = (*sessionServiceDecorator)(nil)` compile check
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-2.1.md`
- Success criteria: as per module task completion protocol

---

**Task 2.2: Add `NewSessionsStorage` factory**
- Open `runtime/internal/session_service_factory_test.go`
- Add failing tests for `NewSessionsStorage`:
  - `type file` → `SaveMetadata/ListMetadata` work; inner session service is `*sessionServiceDecorator`; underlying meta is `*fileSessionMetadataStore`
  - `type database` → `AutoMigrate()` succeeds; `Create(...)` succeeds after migrate; inner meta is `*DatabaseSessionMetadataStore`
  - `type memory` (empty string) → non-nil, `AutoMigrate()` no-op
  - `UseFileStorage=true` flag → file-backed
  - `UseDatabaseStorage=true` flag → database-backed
  - Unknown type string → error
- Run `npx nx test runtime --skipNxCache` — compile errors → add stubs
- Edit `runtime/internal/session_service_factory.go`:
  - Add `NewSessionsStorage(deps SessionServiceFactoryDeps) (SessionsStorage, error)`
  - Switch on deps flags/type → create concrete raw storage (`*FileSessionsStorage` / `*DatabaseSessionsStorage` / `*MemorySessionsStorage`) → wrap with `NewSessionServiceDecorator(raw, deps.Summarizer, deps.RootLogger)` → return
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-2.2.md`
- Success criteria: as per module task completion protocol

---

**Task 3.1: Update `AgentRunnerFactory` and `AgentRunner`**
- Locate all test files that construct `AgentRunnerFactoryDeps` — update `SessionService` + `SessionMetadataStore` fields to `SessionStorage SessionsStorage`
- Run `npx nx test runtime --skipNxCache` — compile errors signal remaining sites
- Edit `runtime/internal/agentrun.go`:
  - `AgentRunnerFactoryDeps`: replace `SessionService session.Service` + `SessionMetadataStore SessionMetadataStore` with `SessionStorage SessionsStorage`
  - `AgentRunnerFactory`: replace `sessionService` + `sessionMetadataStore` with `sessionStorage SessionsStorage`
  - `AgentRunner`: replace `sessionService` + `sessionMetadataStore` with `sessionStorage SessionsStorage`
  - ADK runner config: `runner.Config{SessionService: f.sessionStorage}` — works because `SessionsStorage` embeds `session.Service`
  - Session Get/Create calls: `a.sessionStorage.Get/Create(...)`
  - Metadata calls: `a.sessionStorage.ListMetadata(...)` replaces `a.sessionMetadataStore.List(...)`
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-3.1.md`
- Success criteria: as per module task completion protocol

---

**Task 3.2: Update `Runner`**
- Edit `runtime/agent/runner.go`:
  - `Runner` struct: replace `sessionSvc`, `adkSessionSvc`, `sessionMetadataStore` with `sessionsStorage internal.SessionsStorage`
  - `NewRunner`: call `internal.NewSessionsStorage(...)` instead of `internal.NewSessionServiceFromConfig`; pass `SessionStorage: ss` to `AgentRunnerFactoryDeps`
  - `AutoMigrate()`: `return r.sessionsStorage.AutoMigrate()`
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-3.2.md`
- Success criteria: as per module task completion protocol

---

**Task 4.1: Simplify or remove `AutoMigrateAll`**
- Check for remaining callers of `AutoMigrateAll` after Task 3.2
  - If no callers: delete `runtime/internal/automigrate.go` and its test file (if exists)
  - If callers remain: change signature to `func AutoMigrateAll(services ...AutoMigratable) error` — remove the special-case `session.Service` first arg (that logic now lives in `DatabaseSessionsStorage.AutoMigrate()`)
- Run `npx nx test runtime --skipNxCache` — verify all tests pass
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-4.1.md`
- Success criteria: as per module task completion protocol

---

**Task 5.1: Remove dead code** *(done)*
- Remove from `runtime/internal/session_service_factory.go`:
  - `SessionServiceFromConfigResult` type
  - `NewSessionServiceFromConfig` function
  - `newDecoratedSessionService` helper
  - `adkAndMetadataFromSessionsStorage` (only used by the above)
- Remove matching tests in `session_service_factory_test.go` (tests for `NewSessionServiceFromConfig`)
- Run `npx nx test runtime --skipNxCache` — fix any remaining compile errors
- Run `make affected-lint-test` — verify no lint errors
- Write summary to `runtime/doc/implementation/unified-sessions-storage/summary-task-5.1.md`
- Success criteria: as per module task completion protocol

---

**Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
