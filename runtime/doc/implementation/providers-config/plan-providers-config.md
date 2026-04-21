# Plan: LLM Provider Configuration Management

## 1. Introduction / Overview

Currently, LLM provider configuration is hardcoded in the sonalmod app's config YAML and environment variables (`openai.provider`, `openai.baseURL`, `openai.apiKey`, `openai.defaultModel`). There is no way for users to add, edit, or remove providers at runtime through the API or UI. This is limiting—any change requires restarting the application with updated config.

This plan introduces a **provider configuration management API** (CRUD) in the **runtime module** and a **Providers UI page** in the **sonal-ui module**. The API follows the same patterns as the existing agent-runs/sessions API (OpenAPI spec → oapi-codegen → server handlers → public `httpapi` contract).

**Goal:** Allow users to manage LLM provider configurations (create, read, update, delete) via API and UI.

**Explicit non-goals (deferred to next iteration):**
- Resolving genkit's static provider initialization constraint.
- Injecting stored provider configs into the running genkit instance (the stored configs are not used by the `agent.Runner` yet).
- Model selection/management UI.

## 2. Business Logic

### Provider Config Entity

A **provider config** represents a configured LLM provider endpoint. Each config has:

| Field | Description |
|---|---|
| `name` | **Primary key.** Unique technical name used as the model-name prefix (e.g., `openai`, `openrouter`). Immutable after creation. Must match `^[a-z][a-z0-9-]*$`. |
| `type` | Provider protocol type. Immutable after creation. Only supported value today: `openai-compatible`. |
| `displayName` | Optional human-friendly label (e.g., "OpenAI", "Anthropic via OpenRouter"). |
| `baseUrl` | Base URL of the provider API endpoint. Required. |
| `apiKey` | API key for authentication. Required on create. Stored in plaintext in file storage (acceptable for local/dev use). Never returned in full in API responses—only a masked preview (`apiKeyPreview`, e.g. `sk-...xYz1`). |
| `createdAt` | Server-set ISO 8601 timestamp. |
| `updatedAt` | Server-set ISO 8601 timestamp. |

### Operations

- **List** all provider configs (sorted by `createdAt` ascending).
- **Create** a new provider config. Validates: `name` is unique, required fields present, `name` matches pattern, `type` is a supported value.
- **Get** a single provider config by `name`.
- **Update** a provider config by `name`. `name` and `type` are immutable (not in the update body). `apiKey` is optional on update—omit to keep current value.
- **Delete** a provider config by `name`.

### Constraints

- `name` is the primary key—unique and immutable after creation.
- `type` is immutable after creation.
- API key is never returned in full; responses include `apiKeyPreview` (last 4 characters, prefixed with `...`).

## 3. High-Level Architecture

```
┌─────────────┐     ┌──────────────────────────┐     ┌───────────────────────────┐
│  sonal-ui   │────▶│ runtime/httpapi           │────▶│ runtime/internal/agentapi │
│  (Svelte)   │     │ (public contract)         │     │ (server handlers)         │
│             │     │ NewHandler(...)            │     │ AgentAPIServer            │
│  /providers │     │ ProvidersConfigService     │     │ - ListProviders           │
│  page       │     │ type alias                │     │ - CreateProvider          │
└─────────────┘     └──────────────────────────┘     │ - GetProvider             │
                                                      │ - UpdateProvider          │
                                                      │ - DeleteProvider          │
                                                      └───────────┬───────────────┘
                                                                  │
                                                      ┌───────────▼───────────────┐
                                                      │ runtime/internal          │
                                                      │ ProvidersConfigService    │
                                                      │ (interface)               │
                                                      │                           │
                                                      │ FileProvidersConfigService│
                                                      │ (implementation)          │
                                                      │ stores JSON in dataDir    │
                                                      └───────────────────────────┘
```

### Components Involved

| Component | Module | Role |
|---|---|---|
| `ProviderConfig` type | `runtime/internal` | Domain type |
| `ProvidersConfigService` interface | `runtime/internal` | Storage contract (List, Get, Create, Update, Delete) |
| `FileProvidersConfigService` | `runtime/internal` | File-based JSON implementation |
| OpenAPI spec | `runtime/internal/agentapi/openapi.yaml` | Extended with provider CRUD endpoints |
| `api.gen.go` | `runtime/internal/agentapi` | Regenerated from spec |
| Provider handlers | `runtime/internal/agentapi` | Server handler methods for provider endpoints |
| `httpapi.NewHandler` | `runtime/httpapi` | Extended to accept `ProvidersConfigService` |
| `ProvidersConfigService` alias | `runtime/httpapi` | Public type alias for the internal interface |
| Provider config setup | `apps/sonalmod/internal` | Creates file service, passes to `httpapi.NewHandler` |
| TypeScript types | `apps/sonal-ui/src/lib/agentapi` | Regenerated from updated spec |
| API client functions | `apps/sonal-ui/src/lib/agentapi` | New functions for provider CRUD |
| Providers page | `apps/sonal-ui/src/pages` | New Svelte page for managing providers |
| Nav / routing | `apps/sonal-ui/src` | New route + nav link |

## 4. Detailed Architecture

### 4.1 Runtime Internal — Domain Types & Service Interface

**New file: `runtime/internal/providers_config.go`**

Defines:
- `ProviderConfig` struct with fields: `Name`, `Type`, `DisplayName`, `BaseURL`, `APIKey`, `CreatedAt`, `UpdatedAt`.
- `CreateProviderConfigParams` and `UpdateProviderConfigParams` structs for service method inputs.
- `ProvidersConfigService` interface with methods: `List(ctx) ([]ProviderConfig, error)`, `Get(ctx, name) (*ProviderConfig, error)`, `Create(ctx, params) (*ProviderConfig, error)`, `Update(ctx, name, params) (*ProviderConfig, error)`, `Delete(ctx, name) error`.
- Sentinel errors: `ErrProviderConfigNotFound`, `ErrProviderConfigNameConflict`.
- Constant `ProviderTypeOpenAICompatible = "openai-compatible"`.

### 4.2 Runtime Internal — File-Based Implementation

**New file: `runtime/internal/file_providers_config_service.go`**

Storage layout: `{baseDir}/providers/{name}.json` — one JSON file per provider config. The file name matches the provider name, making storage human-readable and debuggable.

Implementation:
- `NewFileProvidersConfigService(baseDir string, logger *slog.Logger) (*FileProvidersConfigService, error)` — creates `{baseDir}/providers/` directory.
- `List` reads all `.json` files in the providers directory, parses them, sorts by `CreatedAt`.
- `Get` reads `{name}.json`; returns `ErrProviderConfigNotFound` if file does not exist.
- `Create` validates name pattern and uniqueness (file must not exist), validates `type`, writes JSON.
- `Update` reads existing file, applies changes (keeps current `apiKey` if params field is empty), writes back.
- `Delete` removes the JSON file; returns `ErrProviderConfigNotFound` if not found.
- Thread-safe via `sync.RWMutex` (same pattern as `fileSessionService`).

**New file: `runtime/internal/file_providers_config_service_test.go`**

Tests use `t.TempDir()` for isolation, faker for test data.

### 4.3 Runtime API — OpenAPI Spec Extension

**Modified file: `runtime/internal/agentapi/openapi.yaml`**

Add new tag `Providers` and five endpoints under `/providers` and `/providers/{providerName}`:

- `GET /providers` → `listProviders` → 200 `ProviderListResponse`
- `POST /providers` → `createProvider` → 201 `ProviderResponse`
- `GET /providers/{providerName}` → `getProvider` → 200 `ProviderResponse`
- `PUT /providers/{providerName}` → `updateProvider` → 200 `ProviderResponse`
- `DELETE /providers/{providerName}` → `deleteProvider` → 204 No Content

New schemas:
- `CreateProviderRequest` — required: `name`, `type`, `baseUrl`, `apiKey`; optional: `displayName`
- `UpdateProviderRequest` — required: `baseUrl`; optional: `displayName`, `apiKey` (omit to keep current)
- `ProviderResponse` — `name`, `type`, `displayName`, `baseUrl`, `apiKeyPreview`, `createdAt`, `updatedAt`
- `ProviderListResponse` — `{ providers: ProviderResponse[] }`
- `ProviderName` parameter component (path parameter, `minLength: 1`)

All responses follow existing patterns: `ProblemDetails` for errors, `401` for unauthorized, `400` for validation, `404` for not found, `409` for name conflict.

### 4.4 Runtime API — Server Handlers

**Modified file: `runtime/internal/agentapi/server.go`** — Add `providersSvc ProvidersConfigService` field to `AgentAPIServer`, add to `ServerParams`.

**New file: `runtime/internal/agentapi/provider_handlers.go`** — Implements `ListProviders`, `CreateProvider`, `GetProvider`, `UpdateProvider`, `DeleteProvider` methods on `AgentAPIServer`. Each method:
1. Checks authentication (caller identity from context).
2. Validates request body / path params.
3. Delegates to `ProvidersConfigService`.
4. Maps internal types to API response types (including API key masking).
5. Returns appropriate HTTP status and JSON body.

If `providersSvc` is nil (provider management not configured), all provider endpoints return `501 Not Implemented`.

**New file: `runtime/internal/agentapi/provider_handlers_test.go`** — Tests with mocked `ProvidersConfigService`.

**New file: `runtime/internal/agentapi/provider_mapper.go`** — Helper functions to convert between internal `ProviderConfig` and API response types, including `maskAPIKey(key string) string`.

### 4.5 Runtime Public Contract — httpapi Extension

**Modified file: `runtime/httpapi/handler.go`**

- Add `ProvidersConfigService` type alias (from `internal/agentapi` or `internal`).
- Add `ProvidersConfigService` field to `HandlerArgs` (nilable; when nil, provider endpoints return 501).
- Pass the service to `agentapi.NewAgentAPIServer` via `ServerParams`.

### 4.6 Sonalmod App — Wiring

**Modified file: `apps/sonalmod/internal/runtime.go`**

- Create `FileProvidersConfigService` using `deps.DataDir` as base directory.
- Pass it to `httpapi.NewHandler` via `HandlerArgs.ProvidersConfigService`.

### 4.7 UI — API Client Layer

**Regenerated file: `apps/sonal-ui/src/lib/agentapi/agentapi.generated.ts`** — via `make generate-api` after spec changes.

**Modified file: `apps/sonal-ui/src/lib/agentapi/types.ts`** — Re-export new provider-related types.

**Modified file: `apps/sonal-ui/src/lib/agentapi/client.ts`** — Add functions:
- `listProviders()` → GET `/providers`
- `createProvider(body)` → POST `/providers`
- `getProvider(providerName)` → GET `/providers/{providerName}`
- `updateProvider(providerName, body)` → PUT `/providers/{providerName}`
- `deleteProvider(providerName)` → DELETE `/providers/{providerName}`

**New file: `apps/sonal-ui/src/lib/agentapi/client.test.ts`** — extend with provider API tests (MSW handlers).

### 4.8 UI — Providers Page

**New file: `apps/sonal-ui/src/pages/Providers.svelte`**

Layout:
- **Header:** "Providers" title + "Add Provider" button.
- **Provider list/table:** Each row shows `displayName` (or `name` if no displayName), `name` (technical), `type`, `baseUrl`, `apiKeyPreview`, edit/delete action buttons.
- **Empty state:** "No providers configured yet. Add your first provider to get started."
- **Add/Edit form:** Inline or modal form with fields: `name` (text, create-only), `type` (select, create-only, only `openai-compatible` for now), `displayName` (text), `baseUrl` (url input), `apiKey` (password input). Save + Cancel buttons.
- **Delete confirmation:** Simple confirm dialog before delete.
- **Loading/error states:** Loading spinner during API calls, error alert on failure.

**New file: `apps/sonal-ui/src/pages/Providers.test.ts`** — Vitest tests for the page (render, list, create, edit, delete flows).

### 4.9 UI — Routing & Navigation

**Modified file: `apps/sonal-ui/src/App.svelte`** — Add `/providers` route (guarded, same as `/about`).

**Modified file: `apps/sonal-ui/src/components/Nav.svelte`** — Add "Providers" link between "Chat" and "About".

**Modified file: `apps/sonal-ui/ui-wireframe.md`** — Document the new `/providers` route and page layout.

## 5. Key Architectural Decisions

1. **Provider config management is decoupled from provider usage.** The stored configs are not injected into the running genkit/runner instance. This is deferred to a future iteration that addresses genkit's static plugin initialization.

2. **`name` is the primary key (no separate UUID `id`).** The `name` field is already unique, immutable, URL-safe (`^[a-z][a-z0-9-]*$`), and semantically meaningful (it's the model-name prefix like `openai/gpt-4.1`). Using it as the primary key simplifies the model, gives human-readable URLs (`/providers/openai`), eliminates an indirection layer, and makes file storage self-describing (`openai.json`). If name immutability is ever relaxed, an internal `id` can be introduced at that point.

3. **API lives in the runtime module** alongside the existing agent-runs/sessions API. This means the same OpenAPI spec, same HTTP handler, same auth middleware. Provider management is exposed under the same `/api/v1/runtime/` prefix in the sonalmod app.

4. **File-based storage** matches the existing `FileSessionService` pattern. JSON files under `{dataDir}/providers/`. No database dependency added.

5. **API key is never returned in full.** Responses include `apiKeyPreview` (masked). On update, omitting `apiKey` preserves the current value.

6. **`ProvidersConfigService` is optional in `HandlerArgs`.** When nil, provider endpoints return 501. This keeps the runtime library flexible for embedders that don't need provider management.

7. **Provider `type` field.** Each provider has an explicit `type` (e.g., `openai-compatible`) set at creation time and immutable thereafter. This enables future support for non-OpenAI-compatible protocols without changing the storage model.

## 6. Uncertainties

1. **API key storage security:** Plaintext file storage is acceptable for local development but may need encryption or a secret store for production deployments. This is a known limitation documented but not addressed in this iteration.

2. **Name validation pattern (`^[a-z][a-z0-9-]*$`):** This is modeled after the existing `OpenAICompatibleLLMProviderArgs.Name` usage. If genkit/ADK has stricter or different requirements for provider names, the pattern may need adjustment when the injection bridge is built.

## 7. Related Files

### Runtime module (`runtime/`)

| File | Action |
|---|---|
| `internal/providers_config.go` | **New** — domain types + service interface |
| `internal/file_providers_config_service.go` | **New** — file-based implementation |
| `internal/file_providers_config_service_test.go` | **New** — tests |
| `internal/agentapi/openapi.yaml` | **Modified** — add provider endpoints + schemas |
| `internal/agentapi/api.gen.go` | **Regenerated** |
| `internal/agentapi/server.go` | **Modified** — add `providersSvc` to struct + params |
| `internal/agentapi/provider_handlers.go` | **New** — CRUD handler methods |
| `internal/agentapi/provider_handlers_test.go` | **New** — handler tests |
| `internal/agentapi/provider_mapper.go` | **New** — internal ↔ API type mapping + key masking |
| `internal/agentapi/mocks_test.go` | **Modified** — add mock for ProvidersConfigService |
| `httpapi/handler.go` | **Modified** — add ProvidersConfigService to HandlerArgs |

### Sonalmod app (`apps/sonalmod/`)

| File | Action |
|---|---|
| `internal/runtime.go` | **Modified** — create + wire file provider service |

### UI (`apps/sonal-ui/`)

| File | Action |
|---|---|
| `src/lib/agentapi/agentapi.generated.ts` | **Regenerated** |
| `src/lib/agentapi/types.ts` | **Modified** — re-export provider types |
| `src/lib/agentapi/client.ts` | **Modified** — add provider API functions |
| `src/lib/agentapi/client.test.ts` | **Modified** — add provider API tests |
| `src/pages/Providers.svelte` | **New** — providers management page |
| `src/pages/Providers.test.ts` | **New** — page tests |
| `src/App.svelte` | **Modified** — add `/providers` route |
| `src/App.test.ts` | **Modified** — update if route assertions exist |
| `src/components/Nav.svelte` | **Modified** — add Providers link |
| `ui-wireframe.md` | **Modified** — document new route + page |

## 8. Task List

All tasks follow TDD. Each task must leave the codebase in a buildable state per module-specific task completion protocol (lint ✓, tests ✓).

---

**Task 1: Define provider config domain types and service interface**
- Create `runtime/internal/providers_config.go`
- Define `ProviderConfig` struct with fields: `Name`, `Type`, `DisplayName`, `BaseURL`, `APIKey`, `CreatedAt`, `UpdatedAt`
- Define `CreateProviderConfigParams` (fields: `Name`, `Type`, `DisplayName`, `BaseURL`, `APIKey`) and `UpdateProviderConfigParams` (fields: `DisplayName`, `BaseURL`, `APIKey`)
- Define `ProvidersConfigService` interface with `List`, `Get`, `Create`, `Update`, `Delete`
  - `List(ctx) ([]ProviderConfig, error)`
  - `Get(ctx, name string) (*ProviderConfig, error)`
  - `Create(ctx, params CreateProviderConfigParams) (*ProviderConfig, error)`
  - `Update(ctx, name string, params UpdateProviderConfigParams) (*ProviderConfig, error)`
  - `Delete(ctx, name string) error`
- Define sentinel errors: `ErrProviderConfigNotFound`, `ErrProviderConfigNameConflict`
- Define constant `ProviderTypeOpenAICompatible = "openai-compatible"`
- No tests needed (types/interfaces only, no logic)
- Run `make lint` from `runtime/` — verify no errors
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-1.md`

---

**Task 2: Implement file-based ProvidersConfigService**
- Create `runtime/internal/file_providers_config_service.go`
- Implement `NewFileProvidersConfigService(baseDir string, logger *slog.Logger) (*FileProvidersConfigService, error)`
- Implement all five methods: `List`, `Get`, `Create`, `Update`, `Delete`
  - Storage: `{baseDir}/providers/{name}.json`
  - `Create`: validate name pattern (`^[a-z][a-z0-9-]*$`), validate type is supported, check file does not exist (name uniqueness), write JSON
  - `Update`: read existing file, apply changes (preserve `apiKey` if params field is empty), write back
  - `Delete`: remove file, return `ErrProviderConfigNotFound` if not found
  - Thread safety via `sync.RWMutex`
- Create `runtime/internal/file_providers_config_service_test.go`
- Write failing tests first (TDD):
  - **List:** returns empty list when no providers; returns all providers sorted by `createdAt`
  - **Get:** returns provider by name; returns `ErrProviderConfigNotFound` for unknown name
  - **Create:** creates provider and returns it with timestamps; rejects duplicate name (`ErrProviderConfigNameConflict`); rejects invalid name pattern; rejects unsupported type
  - **Update:** updates provider fields; preserves API key when not provided; returns `ErrProviderConfigNotFound` for unknown name
  - **Delete:** deletes provider; returns `ErrProviderConfigNotFound` for unknown name
- Run affected tests: `go test -v ./internal/ --run TestFileProvidersConfigService`
  - Verify failure is expectation-based (not compilation errors)
- Implement the logic
- Run affected tests — verify all pass
- Run `make lint` from `runtime/` — verify no errors
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-2.md`

---

**Task 3: Extend OpenAPI spec with provider endpoints**
- Modify `runtime/internal/agentapi/openapi.yaml`
- Add `Providers` tag
- Add `ProviderName` parameter component (path parameter, `minLength: 1`)
- Add schemas: `CreateProviderRequest`, `UpdateProviderRequest`, `ProviderResponse`, `ProviderListResponse`
  - `CreateProviderRequest`: required `name`, `type`, `baseUrl`, `apiKey`; optional `displayName`
  - `UpdateProviderRequest`: required `baseUrl`; optional `displayName`, `apiKey`
  - `ProviderResponse`: `name`, `type`, `displayName`, `baseUrl`, `apiKeyPreview`, `createdAt`, `updatedAt`
  - `ProviderListResponse`: `{ providers: ProviderResponse[] }`
- Add response `Conflict` (409) with `ProblemDetails`
- Add paths:
  - `GET /providers` → `listProviders` → 200, 401, 500
  - `POST /providers` → `createProvider` → 201, 400, 401, 409, 500
  - `GET /providers/{providerName}` → `getProvider` → 200, 401, 404, 500
  - `PUT /providers/{providerName}` → `updateProvider` → 200, 400, 401, 404, 500
  - `DELETE /providers/{providerName}` → `deleteProvider` → 204, 401, 404, 500
- Property names follow camelCase convention
- Regenerate Go code: `go generate ./internal/agentapi`
- Run `make lint` from `runtime/` — verify no errors (generated code may need handler stubs)
- Note: The generated `ServerInterface` will have new methods. Add stub implementations on `AgentAPIServer` that return 501 so the code compiles. These will be fully implemented in Task 5.
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-3.md`

---

**Task 4: Create provider response mapper and API key masking**
- Create `runtime/internal/agentapi/provider_mapper.go`
- Implement `mapProviderConfigToResponse(cfg internal.ProviderConfig) ProviderResponse` — maps internal type to generated API type, masks API key
- Implement `maskAPIKey(key string) string` — returns `"...XXXX"` (last 4 chars) or `"..."` if key is shorter than 4 chars, or empty string if key is empty
- Implement `mapProviderListToResponse(configs []internal.ProviderConfig) ProviderListResponse`
- Write failing tests in a new `runtime/internal/agentapi/provider_mapper_test.go`:
  - `maskAPIKey` with various key lengths (empty, short, normal)
  - `mapProviderConfigToResponse` maps all fields correctly including `type`
- Run affected tests: `go test -v ./internal/agentapi/ --run TestProviderMapper`
- Implement the logic
- Run affected tests — verify all pass
- Run `make lint` from `runtime/` — verify no errors
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-4.md`

---

**Task 5: Implement provider API server handlers**
- Modify `runtime/internal/agentapi/server.go`:
  - Add `providersSvc` field to `AgentAPIServer` (type: `internal.ProvidersConfigService`, nilable)
  - Add `ProvidersConfigService` field to `ServerParams`
  - Wire in constructor
- Create `runtime/internal/agentapi/provider_handlers.go`:
  - Implement `ListProviders` — delegates to `providersSvc.List`, maps response
  - Implement `CreateProvider` — parse body, validate, delegate to `providersSvc.Create`, return 201
  - Implement `GetProvider` — delegate to `providersSvc.Get`, map response, handle not-found
  - Implement `UpdateProvider` — parse body, delegate to `providersSvc.Update`, handle not-found
  - Implement `DeleteProvider` — delegate to `providersSvc.Delete`, handle not-found, return 204
  - All methods: check caller identity (401 if missing), return 501 if `providersSvc` is nil
  - Map `ErrProviderConfigNotFound` → 404, `ErrProviderConfigNameConflict` → 409
- Remove the stub implementations added in Task 3
- Update mocks: add mock for `ProvidersConfigService` in `runtime/internal/agentapi/mocks_test.go` (use mockery or hand-write per project convention)
- Create `runtime/internal/agentapi/provider_handlers_test.go`:
  - Write failing tests first:
    - `ListProviders`: returns list; returns 501 when service is nil; returns 401 when unauthenticated
    - `CreateProvider`: creates and returns 201; returns 400 for invalid body; returns 409 for duplicate name; returns 401
    - `GetProvider`: returns provider; returns 404 for unknown name; returns 401
    - `UpdateProvider`: updates and returns 200; returns 404; returns 400 for invalid body; returns 401
    - `DeleteProvider`: returns 204; returns 404; returns 401
- Run affected tests: `go test -v ./internal/agentapi/ --run TestProviderHandlers`
  - Verify failures are expectation-based
- Implement the handler logic
- Run affected tests — verify all pass
- Run `make lint` from `runtime/` — verify no errors
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-5.md`

---

**Task 6: Extend public httpapi contract**
- Modify `runtime/httpapi/handler.go`:
  - Add `ProvidersConfigService` type alias from internal (similar to `AgentRunner` alias)
  - Add `ProvidersConfigService` field to `HandlerArgs` (nilable — nil disables provider endpoints with 501)
  - Pass `ProvidersConfigService` to `agentapi.ServerParams` in `NewHandler`
- Update `runtime/httpapi/handler_test.go`:
  - Add test: `NewHandler` works with nil `ProvidersConfigService` (existing behavior unchanged)
  - Add test: `NewHandler` works with non-nil `ProvidersConfigService`
- Run affected tests: `go test -v ./httpapi/ --run TestHandler`
- Run `make lint` from `runtime/` — verify no errors
- Run `make test` from `runtime/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-6.md`

---

**Task 7: Wire provider config service in sonalmod app**
- Modify `apps/sonalmod/internal/runtime.go`:
  - Create `FileProvidersConfigService` using `deps.DataDir` as base directory
  - Pass it to `httpapi.HandlerArgs.ProvidersConfigService`
- Run `make lint` from `apps/sonalmod/` — verify no errors
- Run `make test` from `apps/sonalmod/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-7.md`

---

**Task 8: Regenerate UI TypeScript types and add provider API client functions**
- Run `make generate-api` from `apps/sonal-ui/` to regenerate `agentapi.generated.ts` from the updated spec
- Modify `apps/sonal-ui/src/lib/agentapi/types.ts`:
  - Re-export provider-related types (`ProviderResponse`, `CreateProviderRequest`, `UpdateProviderRequest`, `ProviderListResponse`)
- Modify `apps/sonal-ui/src/lib/agentapi/client.ts`:
  - Add `listProviders()` — GET `/providers`, returns `ProviderListResponse`
  - Add `createProvider(body: CreateProviderRequest)` — POST `/providers`, returns `ProviderResponse`
  - Add `getProvider(providerName: string)` — GET `/providers/{providerName}`, returns `ProviderResponse`
  - Add `updateProvider(providerName: string, body: UpdateProviderRequest)` — PUT `/providers/{providerName}`, returns `ProviderResponse`
  - Add `deleteProvider(providerName: string)` — DELETE `/providers/{providerName}`, returns void/204
- Extend `apps/sonal-ui/src/lib/agentapi/client.test.ts`:
  - Write failing tests first:
    - `listProviders` returns provider list
    - `createProvider` sends correct request and returns response
    - `getProvider` returns single provider
    - `updateProvider` sends correct request
    - `deleteProvider` sends DELETE request
  - Implement (functions are straightforward openapi-fetch wrappers)
  - Verify all tests pass
- Run `make lint` from `apps/sonal-ui/` — verify no errors
- Run `make test` from `apps/sonal-ui/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-8.md`

---

**Task 9: Create Providers page**
- Create `apps/sonal-ui/src/pages/Providers.svelte`:
  - **Header:** "Providers" title + "Add Provider" button
  - **Provider list:** Table/cards showing each provider: `displayName` (or `name`), `name`, `type`, `baseUrl`, `apiKeyPreview`, Edit/Delete buttons
  - **Empty state:** Message when no providers exist
  - **Add/Edit form:** Fields: `name` (text, create-only), `type` (select, create-only, only `openai-compatible` for now), `displayName` (text), `baseUrl` (url input), `apiKey` (password input). Save + Cancel buttons.
  - **Delete flow:** Confirmation before delete
  - **Loading state:** Spinner/indicator during API calls
  - **Error state:** Alert on API errors
  - On mount, call `listProviders()` to load data
  - After create/update/delete, refresh the list
- Create `apps/sonal-ui/src/pages/Providers.test.ts`:
  - Write failing tests first:
    - Renders empty state when no providers
    - Renders provider list
    - Add flow: opens form, submits, list refreshes
    - Edit flow: opens form with current values, submits update
    - Delete flow: confirms and removes provider
    - Shows error on API failure
  - Implement the component
  - Verify all tests pass
- Run `make lint` from `apps/sonal-ui/` — verify no errors
- Run `make test` from `apps/sonal-ui/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-9.md`

---

**Task 10: Update routing, navigation, and wireframe**
- Modify `apps/sonal-ui/src/App.svelte`:
  - Import `Providers` page
  - Add route `/providers` with auth guard (same pattern as `/about`)
- Modify `apps/sonal-ui/src/components/Nav.svelte`:
  - Add "Providers" link between "Chat" and "About"
- Update `apps/sonal-ui/src/App.test.ts` if it has route-related assertions
- Modify `apps/sonal-ui/ui-wireframe.md`:
  - Add `/providers` to the Routes table
  - Add "Providers" section describing the page layout and states
  - Update Nav description to include the new link
- Run `make lint` from `apps/sonal-ui/` — verify no errors
- Run `make test` from `apps/sonal-ui/` — verify all tests pass
- Write summary to `runtime/doc/implementation/providers-config/summary-task-10.md`

---

**Task 11: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
