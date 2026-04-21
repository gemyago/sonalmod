# Implementation Summary: Database Storage Support

**Plan:** [plan-database-storage.md](./plan-database-storage.md)

## Overview

Added SQLite/Postgres database storage backends for both session storage and provider configuration in the runtime module and `sonalmod` app. A shared DSN/dialector helper was extracted, a full `DatabaseProvidersConfigService` with CRUD was implemented, an `AutoMigrateAll` aggregator was added, and the `Runner` public API was extended with `WithDatabaseStorage` and `AutoMigrate`. The `sonalmod` app was wired to select database or file storage via config, and all affected modules passed lint and tests.

## Tasks

### Task 1.1: Extract shared DSN/dialector helper
Created `db_helpers.go` with `isSQLiteDSN` and `newDialector` helpers extracted from `db_session_service.go`. Pure refactor with no logic changes.

### Task 1.2: DatabaseProvidersConfigService — GORM model and constructor
Created `db_providers_config_service.go` with the GORM model (`provider_configs` table), constructor, `AutoMigrate`, and full `List`/`Get` implementations. `Create`/`Update`/`Delete` were left as stubs. `List` and `Get` were implemented (beyond the planned stub scope) to satisfy the 90% per-file coverage threshold; tests insert records directly via the internal `svc.db` field. `providerModelToConfig` uses Go type conversion (`ProviderConfig(m)`) per staticcheck suggestion.

### Task 1.3: DatabaseProvidersConfigService CRUD methods
Implemented `Create`, `Update`, and `Delete`. `Create` validates the name pattern and type and maps `gorm.ErrDuplicatedKey` → `ErrProviderConfigNameConflict`. `Update` fetches the existing record and preserves the API key when the update value is empty. `Delete` checks `RowsAffected == 0` to return `ErrProviderConfigNotFound`. `TranslateError: true` was added to GORM config (not mentioned in plan) to make `errors.Is(err, gorm.ErrDuplicatedKey)` work with the SQLite driver. DB-failure error-path tests (closing underlying SQL connection) were added to meet the 90% coverage threshold.

### Task 1.4: Internal AutoMigrateAll aggregator
Created `automigrate.go` with `AutoMigratable` interface and `AutoMigrateAll` that migrates the session service and any variadic services implementing `AutoMigratable`, collecting errors via `errors.Join`. Skipping a non-database session service required checking `strings.Contains(err.Error(), "invalid session service type")` because the ADK returns an unexported, unwrapped error with no exported sentinel.

### Task 1.5: WithDatabaseStorage and AutoMigrate on Runner public API
Added `WithDatabaseStorage(dsn string) RunnerOpt` with mutual exclusivity against `WithFileSystemStorage` (last-writer-wins). `Runner` now stores `sessionSvc` and exposes `AutoMigrate() error` delegating to `AutoMigrateAll`. The `ProvidersConfigService` variadic argument was intentionally left empty, to be populated in task 1.7.

### Task 1.6: Add NewDatabaseProvidersConfigService to httpapi package
Added `NewDatabaseProvidersConfigService` factory in `providers_config_from.go`, mirroring the existing file factory pattern. No deviations.

### Task 1.7: Wire database storage in apps/sonalmod
Added `agentRuntime.storage.type` and `agentRuntime.database.dsn` config keys (historically documented as `ai.sessionStorage` / `ai.database`). `NewRuntime` selects `WithDatabaseStorage` when type is `"database"`. Extracted a `newProvidersConfigService` helper (unplanned refactor) to keep `NewRuntime` under the 100-line `funlen` limit.

### Task 1.8: Final validation
All 5 affected projects passed lint and tests. Updated `runtime/AGENTS.md` and `apps/sonalmod/AGENTS.md` to document the new public API and config keys.

## Deviations & notes

- **`List`/`Get` implemented in task 1.2** beyond stub scope to meet the 90% per-file coverage threshold; tests use the internal `svc.db` field directly.
- **`TranslateError: true` (task 1.3)**: Required for `gorm.ErrDuplicatedKey` detection with the SQLite driver; not mentioned in the plan.
- **String-matching ADK error (task 1.4)**: `AutoMigrateAll` suppresses the ADK "invalid session service type" error via `strings.Contains` because no exported sentinel is available.
- **`newProvidersConfigService` helper (task 1.7)**: Unplanned extraction required to satisfy the `funlen` linter (function body > 100 lines).

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
