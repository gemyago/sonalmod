# Plan: Extract session persistence into `runtime/internal/sessions`

## 1. Introduction / Overview

**Problem:** Session-related code in the runtime module has grown across many files at the root of [`runtime/internal`](../../../internal): unified storage (`SessionsStorage`), three backends (memory, file, database), metadata stores, a **listing-metadata sync wrapper** (today named like a generic “decorator”), factory wiring, and title summarization (`Summarizer`, `LLMSummarizer`). Everything lives in the catch-all `internal` package, which makes the surface area hard to navigate and encourages unrelated code to accumulate alongside session concerns.

**Goal:** Move **most** session persistence and metadata logic into a dedicated internal package—[`runtime/internal/sessions`](../../../internal/sessions) (Go package name **`sessions`**)—so that `internal` retains orchestration (agent run, HTTP API server, background runner) while session storage is cohesive and discoverable. This is a **structural refactor**: behavior and public contracts of [`runtime/agent`](../../../agent) stay equivalent (type aliases and `Runner` APIs remain stable).

**Non-goals (initial phase):**

- Changing ADK usage or the **exported** shape of `SessionsStorage` / `runtime/agent` beyond relocation and import paths. **Allowed:** internal renames that clarify intent (e.g. metadata-sync constructor and file names in §4.1) where the public embedder API stays the same.
- Moving **streaming / run-result** session projections such as [`session_event.go`](../../../internal/session_event.go) (`SessionEvent`, mapping from `session.Event`)—those are consumed by `RunResult`, SSE, and stream mappers; they can stay in `internal` until a follow-up if desired.
- Expanding the **public** API of the runtime module beyond updating import paths in type aliases (see §5).

---

## 2. Business Logic

- **Unified storage:** `SessionsStorage` continues to embed `google.golang.org/adk/session`.`Service` and add metadata + `AutoMigrate`—same semantics as today ([`sessions_storage.go`](../../../internal/sessions_storage.go)).
- **Backends:** Memory, file, and database implementations keep their current behavior (including metadata-sync–wrapped factory output).
- **Listing metadata sync:** The wrapper around a raw `SessionsStorage` (today `sessionServiceDecorator`) intercepts `Create`, `AppendEvent`, and `Delete` so the **listing index** (`SessionMetadata`: id, title, timestamps) stays aligned with ADK session state; titles come from `Summarizer` on the first user message. **Rename on extraction** to names that say *what* it does (see §4.1 and §5), not “decorator.”
- **Title summarization:** `TruncatingSummarizer` and `LLMSummarizer` stay the implementations used for session titles; they move with the session package because only session wiring uses them today.

---

## 3. High-Level Architecture

```
runtime/internal/                          (orchestration, HTTP, run loop)
├── agentrun.go, background_runner.go      → import sessions.SessionsStorage, types
├── agentapi/                              → import sessions for ListSessions mapping types
└── db_gorm_config.go, db_dialector.go,    → shared DB helpers (see §5)
    db_providers_config_service.go

runtime/internal/sessions/                 (NEW — package sessions)
├── storage.go                             → SessionsStorage interface, AutoMigratable
├── metadata.go                            → SessionMetadata, params, result, SessionMetadataStore
├── metadata_sync.go                       → listing-metadata sync wrapper (rename from session_service_decorator)
├── factory.go                             → NewSessionsStorage, SessionServiceFactoryDeps
├── summarizer.go, llm_summarizer.go       → moved from internal
├── memory.go, file.go, database.go      → backends (+ colocated metadata/index pieces; see §4.1)
└── ...
```

**Consumers to update:** anything that today references `internal.SessionsStorage`, `internal.SessionMetadata`, `internal.NewSessionsStorage`, `internal.NewMemorySessionsStorage`, etc.—including [`runtime/agent/runner.go`](../../../agent/runner.go) (type aliases and `NewRunner` wiring), tests under `runtime/internal/...` and `runtime/agent/...`.

---

## 4. Detailed Architecture

### 4.1 New package layout (`package sessions`)

**Purpose-based rename (replaces “decorator”):** The old `session_serviceDecorator` / `NewSessionServiceDecorator` is not a generic decorator—it **keeps the session listing index in sync** with ADK operations and derives titles. On extraction, rename along these lines (exact identifiers TBD during implementation; behavior unchanged):

| Old (conceptual) | New (recommended) |
|------------------|-------------------|
| `sessionServiceDecorator` | unexported `metadataSyncStorage` (or `listingMetadataSyncStorage`) |
| `NewSessionServiceDecorator` | `NewMetadataSyncStorage` — returns `SessionsStorage`, wrapping the inner storage |

Doc comment template: *“Wraps a [SessionsStorage] and updates listing metadata (and titles via [Summarizer]) on Create, AppendEvent, and Delete.”*

**Simpler file names:** The package is already `sessions`, so drop repeated tokens such as `session(s)_` and redundant `_storage` in filenames where the file’s role is obvious—for example `memory.go` instead of `memory_sessions_storage.go`, `database.go` instead of `db_sessions_storage.go`. Split into an extra file only when a single file would be unwieldy (e.g. large file ADK service + JSON index).

| Concern | Source (today) | Destination |
|--------|------------------|-------------|
| Interface + metadata types | `sessions_storage.go`, `session_metadata.go` | `sessions/storage.go`, `sessions/metadata.go` |
| Listing metadata sync | `session_service_decorator.go` | `sessions/metadata_sync.go` |
| Factory | `session_service_factory.go` | `sessions/factory.go` |
| Memory backend | `memory_sessions_storage.go`, `memory_session_metadata_store.go` | `sessions/memory.go` (or split if needed) |
| File backend | `file_sessions_storage.go`, `file_session_metadata_store.go`, `file_session_service.go` | `sessions/file.go` (+ optional `file_index.go` if size warrants) |
| Database backend | `db_sessions_storage.go`, `db_session_metadata_store.go`, `db_session_service.go` | `sessions/database.go` (+ optional split) |
| Summarizers | `summarizer.go`, `llm_summarizer.go` (+ tests) | `sessions/summarizer.go`, `sessions/llm_summarizer.go` |
| Tests | `*_test.go` paired with moved files | colocated under `sessions/` |

**Where the `SessionsStorage` interface lives (decision):** Keep **`SessionsStorage`** (and related metadata interfaces) **defined in `sessions`**, not “next to a single consumer” in `runtime/internal`.

- **Why:** Several packages depend on the same contract (`agentrun`, `background_runner`, `agent`, tests). Placing the interface beside one consumer would force others to import that consumer for types, or duplicate interfaces—both are worse than a single domain package owning the contract.
- **Why not move the metadata-sync wrapper to `internal`:** It implements `SessionsStorage` by wrapping another `SessionsStorage`; it is tightly coupled to session persistence and `Summarizer`, and the factory today always applies it. Hoisting the wrapper to `runtime/internal` would split “compose raw backend + sync” across packages without a strong benefit until we need **optional** or **alternate** listing policies. Revisit if product rules diverge (e.g. listing without titles).

**Mockery:** [`mock_session_service_test.go`](../../../internal/mock_session_service_test.go) mocks `session.Service`; keep generation in whichever package owns the tests that need it (likely `sessions` with `//go:generate` path updates) per [`.context/mockery.md`](../../../../.context/mockery.md) if applicable.

### 4.2 Shared dependencies (stay in `internal` or small shared helper)

- **`GormSonalmodTablesOpts` / `NewGormConfigForSonalmodTables`** ([`db_gorm_config.go`](../../../internal/db_gorm_config.go)): used by **both** [`db_providers_config_service.go`](../../../internal/db_providers_config_service.go) and database session/metadata code. **Keep** these types and constructors in `runtime/internal` (or extract a neutral `internal/dbconfig` later). The `sessions` package **imports** `runtime/internal` for these types—allowed because `internal/sessions` is under the same module subtree as `internal`.
- **`newDialector`** ([`db_dialector.go`](../../../internal/db_dialector.go)): currently **unexported**, so it cannot be used from another package. **Decision required** (see §5): either **export** a function such as `NewGormDialector(dsn string) gorm.Dialector` on `runtime/internal`, or move dialector logic to a tiny shared package imported by both `internal` (providers) and `internal/sessions`. Do **not** duplicate DSN routing logic.

### 4.3 `runtime/agent` public surface

- Update **type aliases** to point at `.../runtime/internal/sessions` instead of `.../runtime/internal`, e.g. `type SessionMetadata = sessions.SessionMetadata`.
- [`NewRunner`](../../../agent/runner.go) today builds `internal.NewLLMSummarizer` and `internal.SessionServiceFactoryDeps`; switch to `sessions.NewLLMSummarizer`, `sessions.SessionServiceFactoryDeps`, etc.
- No new exported identifiers required if aliases and constructor wiring preserve the same names.

### 4.4 `internal/agentapi`

- [`session_mapper.go`](../../../internal/agentapi/session_mapper.go): change import from `runtime/internal` to `runtime/internal/sessions` for `SessionMetadata` (or keep a single import alias `rt` → `sessions`).

### 4.5 Documentation

- Update [`runtime/AGENTS.md`](../../../AGENTS.md) **only if** the documented paths for session storage or factory behavior change (e.g. “implementations live under `internal/sessions`”).

---

## 5. Key Architectural Decisions

1. **Package name `sessions` (not `session`)** — avoids clashing with `google.golang.org/adk/session` in imports and conversation.
2. **`SessionsStorage` stays in `sessions`** — the interface is a **domain contract** shared by multiple consumers; defining it beside one consumer would worsen coupling. See §4.1.
3. **Rename “decorator” to metadata-sync language** — files/types/constructors should reflect **listing metadata synchronization** (and title derivation), not the Gang-of-Four pattern name. See §4.1.
4. **Keep the metadata-sync wrapper in `sessions` (for this refactor)** — it implements `SessionsStorage` and is always composed in the factory; moving it to `runtime/internal` is deferred until listing/title policy becomes optional or diverges. See §4.1.
5. **Keep DB table-prefix config in `runtime/internal`** — single source of truth shared with provider config DB; `sessions` imports it.
6. **Export or share GORM dialector** — required for a clean split; prefer one exported helper over copy-paste.
7. **Do not move `session_event.go` in phase one** — it is part of the run/streaming model, not persistence.
8. **Preserve TDD and green builds** — each task leaves `make lint` and `make test` passing from [`runtime/`](../../../).

---

## 6. Uncertainties

- **Exact file split** for large backends (e.g. `file.go` vs `file_index.go`) — follow size and clarity; no semantic difference.
- **Summarizer tests and import cycles:** With summarizers, metadata-sync wrapper, and backends colocated in **`sessions`**, tests in that package should import **`runtime/internal` only for shared DB helpers** (GORM opts, dialector)—same direction as production code. Cycles are **unlikely**. Reserve `package sessions_test` or external test packages only if a test truly needs to import both `sessions` and a package that imports `sessions`.
- **Final identifier for the sync constructor** — `NewMetadataSyncStorage` vs `NewWithListingMetadataSync` (trade-off: brevity vs explicit “listing”). Pick one during Task 1.3 and use consistently in factory tests.
- **Mockery output path** after the move: regenerate mocks so package clauses match.

---

## 7. Related Files

**Move or replace (from `runtime/internal/`):**

- `sessions_storage.go`, `session_metadata.go`
- `memory_sessions_storage.go`, `memory_sessions_storage_test.go`
- `memory_session_metadata_store.go`, `memory_session_metadata_store_test.go`
- `file_sessions_storage.go`, `file_sessions_storage_test.go`
- `file_session_metadata_store.go`, `file_session_metadata_store_test.go`
- `file_session_service.go`, `file_session_service_test.go`
- `db_sessions_storage.go`, `db_sessions_storage_test.go`
- `db_session_metadata_store.go`, `db_session_metadata_store_test.go`
- `db_session_service.go`, `db_session_service_test.go`
- `session_service_decorator.go`, `session_service_decorator_test.go` → become `metadata_sync.go` (+ tests) with renamed types/constructors (§4.1)
- `session_service_factory.go`, `session_service_factory_test.go`
- `summarizer.go`, `summarizer_test.go`, `llm_summarizer.go`, `llm_summarizer_test.go`
- `mock_session_service_test.go` (or regenerate in `sessions`)

**Update imports / wiring:**

- `agentrun.go`, `agentrun_test.go`
- `background_runner.go`, `background_runner_test.go`
- `agentapi/session_mapper.go`, `agentapi/session_handlers.go`, `agentapi/session_mapper_test.go`, `agentapi/session_handlers_test.go` (as needed)
- `agent/runner.go`, `agent/runner_test.go`, `agent/tool_test.go` (if references storage)
- `db_dialector.go` (export or refactor per §4.2)
- `runtime/AGENTS.md` (optional)

**New directory:**

- `runtime/internal/sessions/*.go` (+ tests)

**Explicitly out of scope for this plan:**

- `session_event.go`, `session_event_test.go`, `run_result.go` (unless a later task consolidates “session” naming)

---

## 8. Task list (TDD; module completion protocol)

Follow **TDD** where behavior could change: prefer **tests first** for any new exported surface (e.g. dialector helper). After substantive changes under `runtime/`, run **`make lint`** and **`make test`** from **`runtime/`** (see [`runtime/Makefile`](../../../Makefile)). Each numbered task should end with a green module state.

**Task 1.1: Export or share GORM dialector**

- **Problem:** `newDialector` in [`db_dialector.go`](../../../internal/db_dialector.go) is unexported; `internal/sessions` cannot call it from another package.
- Add an **exported** function (e.g. `NewGormDialector(dsn string) gorm.Dialector`) or a shared internal subpackage; update [`db_providers_config_service.go`](../../../internal/db_providers_config_service.go), [`db_session_metadata_store.go`](../../../internal/db_session_metadata_store.go), and [`db_session_service.go`](../../../internal/db_session_service.go) to use it.
- **Tests:** extend or add tests so SQLite vs PostgreSQL routing still matches existing behavior (table-driven using `:memory:` and a postgres-like DSN string if applicable).
- Run `cd /Users/jenya/projects/sonalmod-local/runtime && make lint && make test`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.1.md`.
- Success criteria: as per module task completion protocol (lint ✓, tests ✓).

**Task 1.2: Create `internal/sessions` package — types and interfaces**

- Add `runtime/internal/sessions` with `SessionsStorage`, metadata types, and `SessionMetadataStore` (moved from current files); adjust package clause and imports.
- **Tests:** move or adapt tests that only touch these types if any; otherwise compilation + existing tests passing is enough per project norms.
- Run `make lint` && `make test` in `runtime/`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.2.md`.
- Success criteria: as per module task completion protocol.

**Task 1.3: Move summarizers and listing-metadata sync wrapper**

- Move `summarizer.go`, `llm_summarizer.go` (+ tests) and `session_service_decorator.go` (+ tests) into `sessions` as `metadata_sync.go` (or split per §4.1); **rename** types/constructors from “decorator” / `NewSessionServiceDecorator` to purpose-based names (e.g. `NewMetadataSyncStorage` — see §4.1 table). Update factory and all call sites.
- **Tests:** same coverage as today for create/append/delete metadata and title behavior; run the full former decorator test file against the new names.
- Run `make lint` && `make test`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.3.md`.
- Success criteria: as per module task completion protocol.

**Task 1.4: Move memory and file backends**

- Move memory and file storage + metadata stores + `file_session_service` (+ all associated tests); use simpler filenames per §4.1 (`memory.go`, `file.go`, …).
- **Tests:** run `go test ./internal/sessions/... -count=1` and full `make test`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.4.md`.
- Success criteria: as per module task completion protocol.

**Task 1.5: Move database backends**

- Move DB sessions storage, DB metadata store, DB session service (+ tests) to e.g. `database.go`; ensure GORM opts and dialector imports resolve.
- Run targeted DB tests, then `make lint` && `make test`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.5.md`.
- Success criteria: as per module task completion protocol.

**Task 1.6: Move factory and wire consumers**

- Move `session_service_factory.go` (+ tests) to `sessions`; update `agentrun.go`, `background_runner.go`, `agent/runner.go`, `agentapi/session_mapper.go`, and any remaining references from `internal` to `sessions`.
- Regenerate mocks if mockery paths changed.
- **Tests:** `go test ./...` from `runtime/` (or `make test`).
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.6.md`.
- Success criteria: as per module task completion protocol.

**Task 1.7: Delete stale files and polish docs**

- Remove empty or duplicated files from old locations; ensure no duplicate symbols in `internal`.
- Update [`runtime/AGENTS.md`](../../../AGENTS.md) if the “internal implementation” narrative should mention `internal/sessions`.
- Run `make lint` && `make test`.
- Write summary to `runtime/doc/implementation/session-package-extraction/summary-task-1.7.md`.
- Success criteria: as per module task completion protocol.

**Compress implementation summaries**

- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries under `runtime/doc/implementation/session-package-extraction/`.

---

## 9. Acceptance criteria (overall)

- `cd runtime && make lint` — no issues.
- `cd runtime && make test` — all pass.
- Session behavior unchanged: listing, persistence, DB migration, and listing-metadata sync (titles) behave as before (covered by existing tests).
- `runtime/agent` public types remain stable via aliases to `sessions` types.
