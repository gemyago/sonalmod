# AI Agent foundational runtime

The module is based on ADK with genkit as the LLM adapter. Module follows standard project structure and best practices.

Language: Go (1.26.x)

Key architectural decisions:
- ADK/genkit related components are ONLY used internally and NOT exposed via public contract.
- Public contract stays minimal and tight.

## Public contract

This is a brief overview of the public contract.

Currently the public contract includes:
- Agent-related components [agent/](./agent/) (including `Runner` options such as `WithFileSystemStorage` for on-disk state under a base directory, `WithDatabaseStorage(dsn)` for database-backed session state, optional `WithDatabaseTablePrefix` for SQL table name prefix when using database storage, and `WithSystemPromptFragments` for extra system prompt sections after the default assistant fragment; see `SystemPromptFragment`)
- `RunnerArgs`: required `ProvidersConfigService` and required `AgentProfilesService` (not optional); `Runner.Run` stays thin while internal runtime execution code owns direct/profile path selection using the supplied profile service; session titles use the first provider model marked for summarization (see OpenAPI `ModelConfig.summarization`) with truncation fallback when none is designated (wired internally, not configurable on `RunnerArgs`)
- `NewRunParams` / `RunParamsBuilder.WithText` in [agent/runparams.go](./agent/runparams.go) — build `RunParams` with user ID, session ID, and fully qualified model (`provider/model-name`), then attach a single text user message for `Runner.Run` (the low-level runner still requires a model on each call; standard HTTP run endpoints accept optional `profileName` plus optional request `model`, requiring at least one)
- Provider configuration persistence: `ProvidersConfigService` and `NewFileProvidersConfigService` / `NewDatabaseProvidersConfigService(dsn, logger, tablePrefix)` in [agent/](./agent/) (constructors delegate to [internal/llmproviders](./internal/llmproviders); file- or database-backed storage for LLM provider settings; empty `tablePrefix` means no GORM table name prefix). Domain types and the service interface live in `llmproviders`; [agent](./agent/) aliases them for the public contract.
- `RunnerArgs.ProvidersConfigService` — **required**; `NewRunner` wires a `ModelsLocator` so providers and models are resolved at run time from this service
- `runner.ModelsLocator()` — returns a `ModelsLister` suitable for passing to `httpapi.HandlerArgs.ModelsLister`; exposes `ListModels` for the `GET /models` endpoint (typically non-nil for runners built with `NewRunner`)
- `agent.ModelsLister` interface and `agent.ModelInfo` type — used to wire models listing between `Runner` and the HTTP handler layer without exposing internal packages
- `Runner.AutoMigrate()` — runs database schema migrations for the configured session storage (ADK session tables and session metadata when using database storage); safe to call when file or in-memory storage is in use (no-op for non-DB backends)
- `AgentRunner` — `Run`, `ReadSession`, and `ListSessions` ([agent/runner.go](./agent/runner.go)); `ListSessions` returns paginated `SessionMetadata` (session id, title, timestamps) for the authenticated user
- `SessionMetadata`, `ListSessionsParams`, `ListSessionsResult` — types for session listing (aliases of internal listing types)
- HTTP API for agent execution [httpapi/](./httpapi/) (`NewHandler` and related types; `HandlerArgs.Runner` is [`agent.AgentRunner`](./agent/runner.go) — typically `*agent.Runner`; `agent.Runner` delegates run-path selection to internal runtime execution code; standard `POST /agent-runs` and `POST /sessions/{sessionId}/agent-runs` requests accept optional `profileName` and optional request `model` (at least one required), regular profiles allow request `model` overrides, and `acp-stdio` profiles ignore request `model`; public `/opencode-*` routes remain removed; `agent.ProvidersConfigService` and optional `agent.ModelsLister` (from `runner.ModelsLocator()`) enable provider and `GET /models` endpoints; the handler wraps the runner for background runs and unified ReadSession)
- HTTP API profile management via `agent-profiles` endpoints in [internal/agentapi/openapi.yaml](./internal/agentapi/openapi.yaml); `httpapi.HandlerArgs` requires `agent.AgentProfilesService` and `agent.ProvidersConfigService` and keeps CRUD logic in internal handlers

Before extending the public contract (e.g. new exported types or methods):
- Prefer unexported helpers or internal packages.
- If export seems necessary, reconsider; unexport if there is any doubt.
- Only export after that second pass, and keep the API minimal.

Rules for doc comments on public contract types and methods:
- Docs should not expose internal implementation details or underlying frameworks used (e.g ADK, genkit e.t.c)
- Docs should be concise and to the point.

## Session persistence

Session storage implementations (memory, file, database backends), listing-metadata sync, and `NewSessionsStorage` (returns concrete `*sessions.MetadataSyncStorage`, which implements `SessionsStorage`) live in [internal/sessions](./internal/sessions). Unified storage interfaces (`SessionsStorage`, `AutoMigratable`) are defined there; callers import `sessions` directly rather than via re-exports on [internal](./internal). LLM provider configuration types and storage (`ProvidersConfigService`, file/DB implementations) live in [internal/llmproviders](./internal/llmproviders). The `Summarizer` interface and implementations (`TruncatingSummarizer`, `LLMSummarizer`) live in [internal/summarize](./internal/summarize). Shared GORM DSN routing and table-prefix config used by session DB code and provider-config DB live in [internal/gormsonal](./internal/gormsonal). The [internal](./internal) package may keep type aliases for some session listing types and `llmproviders` types where needed for orchestration and tests.

## API Layer

Internal implementation: [internal/agentapi](./internal/agentapi)

`ServerInterface` is implemented by **`AgentAPIServer`** in [`internal/agentapi/server.go`](./internal/agentapi/server.go) (dependency-injected runner, logger, id generator, request mapper, SSE writer).

Generated with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen). To regenerate after modifying the spec:
```sh
go generate ./internal/agentapi
```
Note: There is an issue with deps incompatibility that may lead to openapi-codegen fail. If this happens, try to install it from main:
`go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@main` and re-run the generate command.

Public interface: [httpapi/handler.go](./httpapi/handler.go) - must stay small and tight. Embedders pass an [`agent.AgentRunner`](./agent/runner.go) (usually [`*agent.Runner`](./agent/runner.go)) into [`httpapi.NewHandler`](./httpapi/handler.go) via [`HandlerArgs`]; the handler applies the background-runner adapter internally, while internal runtime execution code owns path selection and mode-specific setup.

The generated OpenAPI surface includes `GET /sessions` (pagination: required `limit`, optional `offset`) for listing session metadata, profile CRUD with mode-specific `executionSettings` (`regular` by default, `acp-stdio` when explicitly selected), and standard run requests that carry `message` plus optional `profileName` and optional request `model` (at least one selector required). Provider `ModelConfig` includes optional `summarization` to designate a model for title generation.

## Module Rules and Conventions

This section defines module-specific rules and conventions. Project-level rules and conventions must also be followed.

The rules are:
- Update module rules and conventions when user corrects the behavior of AI.
- OpenAPI JSON uses camelCase for property names or any other identifiers or keys; regenerate after spec edits.
- Tests: Mock for [ProvidersConfigService](./agent/) is generated in [agent/mocks_providers_config.go](./agent/mocks_providers_config.go) (`//go:build !release`) so packages outside `agent` (e.g. `httpapi` tests) can use `agent.NewMockProvidersConfigService`; regenerate with `go run github.com/vektra/mockery/v3` from the `runtime/` module root.
- Tests: Gorm models should always have explicit column names; we do not relay on gorm conventions.

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.

After **any** code or config change in this module (including `internal/llmproviders/`), from the **repository root** run `make affected-lint-test` and fix failures before reporting the task done. Also follow the **Coding Task Completion Protocol** in [../AGENTS.md](../AGENTS.md) (lint/test + AGENTS updates when commands or architecture change).
