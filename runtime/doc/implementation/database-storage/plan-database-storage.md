# Plan: Database Storage Support

## Introduction/Overview

Currently the runtime module supports two session storage backends (in-memory and file-based) but only file-based storage is configurable from the upper `sonalmod` app. The `ProvidersConfigService` only has a file-based implementation. This plan adds:

1. **Database session service configuration** -- expose the existing internal database session service through the `Runner` public API and wire it in the `sonalmod` app config, keeping file storage as the default.
2. **AutoMigrate exposure** -- the ADK `session/database` package exposes `AutoMigrate(session.Service) error`; the `Runner` should expose a public `AutoMigrate()` method so consumers can run schema migrations without importing internal packages.
3. **Database-backed `ProvidersConfigService`** -- a new GORM-based implementation of `ProvidersConfigService` that uses the same database connectivity pattern as the ADK session store, with an `AutoMigrate` function for schema setup. Unit tests use the same SQLite in-memory driver already used by `db_session_service_test.go`.

## Business Logic

### Database session service from config

The internal `SessionServiceFactoryDeps` already supports `SessionStorageType: "database"` and `DatabaseDSN`, but the public `Runner` API only exposes `WithFileSystemStorage`. A new `WithDatabaseStorage(dsn)` option is needed so the `sonalmod` app can switch to database sessions via config. File storage remains the default.

### AutoMigrate

`database.AutoMigrate(session.Service) error` from `google.golang.org/adk/session/database` runs GORM `AutoMigrate` on the session tables. The `Runner` needs a public `AutoMigrate() error` method that:
- Calls ADK `database.AutoMigrate` on the session service (only when database storage is configured; returns nil otherwise).
- Calls the providers config service `AutoMigrate` (only when it is a database-backed implementation; returns nil otherwise).

This way consumers call `runner.AutoMigrate()` once and all database-backed stores get migrated.

### Database ProvidersConfigService

A new `DatabaseProvidersConfigService` implements the existing `ProvidersConfigService` interface using GORM. It stores `ProviderConfig` records in a `provider_configs` table. The implementation mirrors the file-based service's validation (name pattern, type check, conflict detection) but uses GORM for persistence.

## High Level Architecture

```
runtime/
├── agent/
│   └── runner.go            # Add WithDatabaseStorage, AutoMigrate
├── internal/
│   ├── db_session_service.go         # Existing (unchanged)
│   ├── db_providers_config_service.go     # NEW: GORM-based ProvidersConfigService
│   ├── db_providers_config_service_test.go # NEW: tests (SQLite :memory:)
│   ├── providers_config.go           # Existing interface (unchanged)
│   └── automigrate.go                # NEW: internal AutoMigrate aggregator
└── httpapi/
    └── providers_config_from.go      # Add NewDatabaseProvidersConfigService factory

apps/sonalmod/
├── internal/
│   ├── runtime.go                    # Wire database storage option + AutoMigrate
│   └── config/
│       ├── default.yaml              # Add agentRuntime.storage and database config
│       └── provide.go                # Provide new config values into DI
```

## Detailed Architecture

### 1. `runtime/agent/runner.go` -- New `WithDatabaseStorage` option

Add a new `RunnerOpt`:

```go
func WithDatabaseStorage(dsn string) RunnerOpt
```

When set, the runner creates the session service via `internal.NewDatabaseSessionService(dsn)` instead of the file or in-memory service. `WithDatabaseStorage` and `WithFileSystemStorage` are mutually exclusive; the last one set wins. Store the DSN and a `useDatabaseStorage` flag in `runnerOpts`.

Also store the created `session.Service` on the `Runner` struct so `AutoMigrate` can reach it.

### 2. `runtime/agent/runner.go` -- `AutoMigrate()` method

```go
func (r *Runner) AutoMigrate() error
```

This method:
- Calls the internal `AutoMigrateAll` helper passing the session service and providers config service.
- Returns a joined error (or nil).

Doc comment must not mention ADK/GORM internals.

### 3. `runtime/internal/automigrate.go` -- Internal aggregator

```go
// AutoMigratable is implemented by services that support database schema migration.
type AutoMigratable interface {
    AutoMigrate() error
}

func AutoMigrateAll(services ...any) error
```

Iterates over the provided services. For each one:
- If it implements `AutoMigratable`, call `AutoMigrate()`.
- If it is a `session.Service` from the database backend, call `database.AutoMigrate(svc)`.
- Otherwise skip.

Returns `errors.Join` of all errors.

### 4. `runtime/internal/db_providers_config_service.go` -- New GORM implementation

Struct:

```go
type DatabaseProvidersConfigService struct {
    db     *gorm.DB
    logger *slog.Logger
}
```

GORM model:

```go
type providerConfigModel struct {
    Name        string    `gorm:"primaryKey;size:255"`
    Type        string    `gorm:"size:50;not null"`
    DisplayName string    `gorm:"size:255"`
    BaseURL     string    `gorm:"size:2048;not null"`
    APIKey      string    `gorm:"size:2048;not null"`
    CreatedAt   time.Time `gorm:"autoCreateTime"`
    UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (providerConfigModel) TableName() string { return "provider_configs" }
```

Constructor:

```go
func NewDatabaseProvidersConfigService(dsn string, logger *slog.Logger) (*DatabaseProvidersConfigService, error)
```

Uses the same `isSQLiteDSN` / dialector logic from `db_session_service.go` (extract to shared helper if not already).

Methods implement `ProvidersConfigService`:
- `List` -- `db.Order("created_at ASC").Find(&models)`
- `Get` -- `db.First(&model, "name = ?", name)` with `ErrRecordNotFound` mapping
- `Create` -- validate name pattern + type, then `db.Create(&model)` with unique constraint conflict mapping
- `Update` -- `db.First` + selective updates + `db.Save`
- `Delete` -- `db.Delete` with not-found check

Implements `AutoMigratable`:

```go
func (s *DatabaseProvidersConfigService) AutoMigrate() error {
    return s.db.AutoMigrate(&providerConfigModel{})
}
```

### 5. `runtime/httpapi/providers_config_from.go` -- Public factory

Add:

```go
func NewDatabaseProvidersConfigService(dsn string, logger *slog.Logger) (ProvidersConfigService, error)
```

Delegates to `rt.NewDatabaseProvidersConfigService`.

### 6. `apps/sonalmod/internal/config/default.yaml` -- Config additions

```yaml
# Where the embedded agent runtime persists state (sessions, providers config).
# storage.type: "file" (default) or "database"
agentRuntime:
  storage:
    type: file
    file:
      baseDir: ""  # empty = use dataDir
  database:
    dsn: ""  # Provided via env APP_AGENTRUNTIME_DATABASE_DSN
```

### 7. `apps/sonalmod/internal/config/provide.go` -- DI bindings

Add config values:
- `agentRuntime.storage.type` -> `config.agentRuntime.storage.type` (string)
- `agentRuntime.database.dsn` -> `config.agentRuntime.database.dsn` (string)

### 8. `apps/sonalmod/internal/runtime.go` -- Wiring

Add new DI fields to `RuntimeDeps`:

```go
AgentRuntimeStorageType string `name:"config.agentRuntime.storage.type"`
AgentRuntimeDatabaseDSN string `name:"config.agentRuntime.database.dsn"`
```

In `NewRuntime`:
- When `AgentRuntimeStorageType == "database"`, use `agent.WithDatabaseStorage(deps.AgentRuntimeDatabaseDSN)` instead of `agent.WithFileSystemStorage`.
- When `AgentRuntimeStorageType == "database"`, create `ProvidersConfigService` via `httpapi.NewDatabaseProvidersConfigService` instead of `httpapi.NewFileProvidersConfigService`.
- Call `runner.AutoMigrate()` after creating the runner when database storage is configured.

## Key Architectural Decisions

1. **`WithDatabaseStorage` vs extending `WithFileSystemStorage`** -- A separate option is cleaner and makes the API self-documenting. Mutual exclusivity is enforced by last-writer-wins semantics.
2. **`AutoMigrate` on Runner** -- Centralizes migration for all database-backed stores behind one public call. Avoids consumers needing to import internal packages.
3. **`AutoMigratable` interface** -- Decouples the migration aggregator from concrete types. Any new database-backed service just needs to implement this interface.
4. **Shared `isSQLiteDSN` + dialector helper** -- Avoids duplicating DSN-to-dialector logic between session and providers config services. Extract a small internal helper (`openDB` or `newDialector`).
5. **Same SQLite driver for tests** -- Reuse `github.com/glebarez/sqlite` with `:memory:` DSN for unit tests, consistent with `db_session_service_test.go`.
6. **File storage remains default** -- No breaking change for existing deployments.

## Uncertainties

1. **Config key naming** -- The plan originally used `ai.sessionStorage.type` and `ai.database.dsn`; the shipped names are `agentRuntime.storage.type` and `agentRuntime.database.dsn` (see `apps/sonalmod/AGENTS.md` for env mapping).
2. **Shared GORM `*gorm.DB` instance** -- When both session and providers config use database storage, they currently open separate GORM connections (via separate `gorm.Open` calls with the same DSN). For efficiency, a shared `*gorm.DB` could be used, but this adds complexity. The plan keeps separate connections for simplicity; this can be optimized later.

## Related Files

### Existing files to modify
- `runtime/agent/runner.go` -- Add `WithDatabaseStorage`, `AutoMigrate`, store session svc reference
- `runtime/httpapi/providers_config_from.go` -- Add `NewDatabaseProvidersConfigService` factory
- `apps/sonalmod/internal/runtime.go` -- Wire database storage option + AutoMigrate call
- `apps/sonalmod/internal/runtime_test.go` -- Update tests for new config paths
- `apps/sonalmod/internal/config/default.yaml` -- Add `agentRuntime` config section
- `apps/sonalmod/internal/config/provide.go` -- Add DI bindings for new config values

### New files to create
- `runtime/internal/db_providers_config_service.go` -- GORM-based ProvidersConfigService
- `runtime/internal/db_providers_config_service_test.go` -- Tests (SQLite :memory:)
- `runtime/internal/automigrate.go` -- Internal AutoMigrate aggregator
- `runtime/internal/automigrate_test.go` -- Tests for aggregator
- `runtime/internal/db_helpers.go` -- Shared DSN/dialector helper (extracted from `db_session_service.go`)

### Existing files for reference (no changes)
- `runtime/internal/providers_config.go` -- Interface definition
- `runtime/internal/file_providers_config_service.go` -- File implementation (reference for validation logic)
- `runtime/internal/file_providers_config_service_test.go` -- Test patterns to follow
- `runtime/internal/db_session_service.go` -- DSN/dialector pattern to reuse
- `runtime/internal/session_service_factory.go` -- Existing factory pattern

## Task List

TDD approach must be followed for all implementation tasks. Each task leaves the codebase in a buildable/passing state per the module task completion protocol.

**Task 1.1: Extract shared DSN/dialector helper**
- Move `isSQLiteDSN` and the SQLite/Postgres dialector logic from `db_session_service.go` into a new `db_helpers.go` file
- Update `db_session_service.go` to use the extracted helper
- No new logic, no new tests needed (existing tests cover this)
- Run affected tests: `npx nx test runtime --skipNxCache`
- Verify all existing tests still pass
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.1.md`
- All checks from completion protocol must be passed

**Task 1.2: Implement `DatabaseProvidersConfigService` -- GORM model and constructor**
- Create `runtime/internal/db_providers_config_service.go`
- Define `providerConfigModel` GORM model with `TableName() = "provider_configs"`
- Implement `NewDatabaseProvidersConfigService(dsn string, logger *slog.Logger)` constructor using the shared dialector helper
- Implement `AutoMigrate() error` method on the struct
- Add compile-time interface check `var _ ProvidersConfigService = (*DatabaseProvidersConfigService)(nil)`
- Add stub methods that return `errors.New("not implemented")` so it compiles
- Write failing tests in `db_providers_config_service_test.go`:
  - constructor creates service with `:memory:` SQLite
  - constructor fails with invalid DSN
  - AutoMigrate succeeds
- Compilation errors are **not acceptable** -- add stubs first
- Run affected tests: `npx nx test runtime --skipNxCache`
  - Verify failure is expectation-based (not compilation errors)
- Implement constructor and AutoMigrate logic
- Run affected tests and verify they pass
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.2.md`
- All checks from completion protocol must be passed

**Task 1.3: Implement `DatabaseProvidersConfigService` -- CRUD methods**
- Write failing tests for all CRUD operations (mirror the test cases from `file_providers_config_service_test.go`):
  - `List`: empty result, multiple providers sorted by `created_at`
  - `Get`: found, not found (`ErrProviderConfigNotFound`)
  - `Create`: success with timestamps, persistence via Get, name conflict (`ErrProviderConfigNameConflict`), invalid name pattern, invalid type
  - `Update`: field updates, API key preservation when empty, not found error
  - `Delete`: success, not found error
- Run affected tests: `npx nx test runtime --skipNxCache`
  - Verify failures are expectation-based
- Implement `List`, `Get`, `Create`, `Update`, `Delete` methods
  - Reuse `providerNamePattern` and `ProviderTypeOpenAICompatible` validation from `providers_config.go` / `file_providers_config_service.go`
  - Map GORM `ErrRecordNotFound` to `ErrProviderConfigNotFound`
  - Map unique constraint violation to `ErrProviderConfigNameConflict`
- Run affected tests and verify all pass
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.3.md`
- All checks from completion protocol must be passed

**Task 1.4: Implement internal `AutoMigrateAll` aggregator**
- Create `runtime/internal/automigrate.go`
- Define `AutoMigratable` interface with `AutoMigrate() error`
- Implement `AutoMigrateAll(sessionSvc session.Service, services ...any) error`
  - For `sessionSvc`: call `database.AutoMigrate(sessionSvc)` (ignore error if it's not a database service)
  - For each service in `services`: if it implements `AutoMigratable`, call `AutoMigrate()`
  - Return `errors.Join` of all errors
- Write failing tests in `automigrate_test.go`:
  - All database services migrated successfully
  - Non-database session service skipped gracefully
  - Non-migratable service skipped gracefully
  - Error from one service does not prevent others from running
- Run affected tests: `npx nx test runtime --skipNxCache`
  - Verify failures are expectation-based
- Implement the logic
- Run affected tests and verify all pass
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.4.md`
- All checks from completion protocol must be passed

**Task 1.5: Add `WithDatabaseStorage` and `AutoMigrate` to `Runner` public API**
- Add `WithDatabaseStorage(dsn string) RunnerOpt` in `runtime/agent/runner.go`
  - Sets `useDatabaseStorage = true` and `databaseDSN = dsn` on `runnerOpts`
  - Clears `useFileStorage` to enforce mutual exclusivity
- Update `NewRunner` to create `internal.NewDatabaseSessionService(dsn)` when `useDatabaseStorage` is set
- Store the `session.Service` and `ProvidersConfigService` references on the `Runner` struct (needed for `AutoMigrate`)
- Add `AutoMigrate() error` method on `Runner` that delegates to `internal.AutoMigrateAll`
- Doc comments must not mention ADK/GORM internals
- No new test file needed -- existing `runner_test.go` patterns (if any) should be extended, or integration is tested via `apps/sonalmod` tests
- Run affected tests: `npx nx test runtime --skipNxCache`
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.5.md`
- All checks from completion protocol must be passed

**Task 1.6: Add `NewDatabaseProvidersConfigService` to `httpapi` package**
- Add `NewDatabaseProvidersConfigService(dsn string, logger *slog.Logger) (ProvidersConfigService, error)` in `runtime/httpapi/providers_config_from.go`
- Delegates to `rt.NewDatabaseProvidersConfigService`
- Run affected tests: `npx nx test runtime --skipNxCache`
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.6.md`
- All checks from completion protocol must be passed

**Task 1.7: Wire database storage in `apps/sonalmod`**
- Add config to `apps/sonalmod/internal/config/default.yaml`:
  ```yaml
  agentRuntime:
    storage:
      type: file
    database:
      dsn: ""
  ```
- Add DI bindings in `apps/sonalmod/internal/config/provide.go`:
  - `agentRuntime.storage.type` as string
  - `agentRuntime.database.dsn` as string
- Update `RuntimeDeps` in `apps/sonalmod/internal/runtime.go`:
  - Add `AgentRuntimeStorageType string` and `AgentRuntimeDatabaseDSN string`
- Update `NewRuntime`:
  - When `AgentRuntimeStorageType == "database"`, use `agent.WithDatabaseStorage(deps.AgentRuntimeDatabaseDSN)` instead of `agent.WithFileSystemStorage`
  - When `AgentRuntimeStorageType == "database"`, use `httpapi.NewDatabaseProvidersConfigService` instead of `httpapi.NewFileProvidersConfigService`
  - Call `runner.AutoMigrate()` when database storage is configured
  - Default behavior (file storage) remains unchanged
- Update/add tests in `apps/sonalmod/internal/runtime_test.go` if applicable
- Run affected tests: `npx nx test sonalmod --skipNxCache`
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.7.md`
- All checks from completion protocol must be passed

**Task 1.8: Final validation**
- Run `make affected-lint-test` from repo root
- Verify all lint and tests pass across all modules
- Update AGENTS.md files if any commands, workflows, or architecture changed (e.g. document `WithDatabaseStorage` in runtime's public contract section)
- Write summary to `runtime/doc/implementation/database-storage/summary-task-1.8.md`
- All checks from completion protocol must be passed

**Task 1.9: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
