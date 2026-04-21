# Plan: Dynamic Providers and Model Selection

## 1. Introduction / Overview

Currently, LLM providers are hardcoded at startup via static YAML config (`openai.*` keys in `default.yaml` / `local-user.yaml`). A `ProvidersConfigService` CRUD API exists and works (UI + persistence), but it is **disconnected** from the actual genkit runtime that handles agent runs. The genkit framework is initialized once via `genkit.Init()` in `agent.NewRunner()` and does not support reinitialisation.

**Goal:** Make providers fully dynamic. Changes saved via the Providers CRUD API must reflect in agent runs immediately. A user must be able to select a model when starting an agent run. The integration test CLI must also support provider config and model selection.

**Core problem:** `genkit.Init()` is called once and produces an immutable `*genkit.Genkit` instance. Changing providers (adding/removing/updating API keys or base URLs) requires creating a new genkit instance.

**Solution:** Introduce a `ModelsLocator` in the runtime that:
1. Reads from `ProvidersConfigService` to get current provider configs.
2. Caches a `*genkit.Genkit` instance **per provider** (keyed by provider name + `UpdatedAt` timestamp).
3. On each model resolution, checks if cached instance is stale (provider config changed) and reinitializes if needed.
4. Re-registers genkit tool stubs when a new genkit instance is created.

## 2. Business Logic

### Provider-Model Lifecycle
- When a user creates/updates/deletes a provider via the CRUD API, the change is persisted immediately (already works).
- When an agent run starts (or continues), the `model` field from the request determines which provider to use. The model identifier format is `provider-name/model-name` (e.g., `litellm/gpt-4.1`).
- The `ModelsLocator` extracts the provider prefix, looks up the provider config from `ProvidersConfigService`, and either reuses a cached genkit instance or creates a new one.
- Tools stubs must be registered on every new genkit instance (cheap operation -- schema registration only).

### Provider Models Configuration
- Each provider can have a list of configured models (manually specified for now; auto-discovery is a future enhancement).
- The API exposes an endpoint to list available models per provider (reads from provider config).
- The UI shows a model picker dropdown in the Chat page.

### Integration Tests
- The `integration-cli` gains `--providers-config` (path to JSON/YAML config file) and `--model` (fully qualified model name) flags.
- Static provider config from `openai.*` config keys is removed from the main app's runtime wiring -- providers come exclusively from `ProvidersConfigService`.
- Agent integration tests use a config file + model parameter to run against real LLMs.

## 3. High-Level Architecture

```
                    ┌──────────────────────────────┐
                    │     ProvidersConfigService    │
                    │   (existing CRUD, file/DB)    │
                    └───────────────┬───────────────┘
                                    │ reads
                    ┌───────────────▼───────────────┐
                    │        ModelsLocator          │
                    │  - resolveModel(fqModelName)  │
                    │  - cache: map[providerName]   │
                    │      → {genkit, updatedAt}    │
                    │  - listModels()               │
                    └───────────────┬───────────────┘
                                    │ produces model.LLM
                    ┌───────────────▼───────────────┐
                    │    agent.Runner / AgentRun     │
                    │  uses LLM from ModelsLocator  │
                    └───────────────────────────────┘
```

Components involved:
1. **`runtime/internal` -- `ModelsLocator`** (new): Core component that caches genkit instances per provider and resolves models.
2. **`runtime/agent` -- Public API changes**: Expose `ModelsLocator` concept to embedders; update `Runner` to use it instead of single static genkit instance.
3. **`runtime/internal/agentapi`** -- API changes: Wire model field through to `RunParams`; add models listing endpoint.
4. **`runtime/internal` -- `ProviderConfig`**: Extend with `Models` field.
5. **`apps/sonalmod/internal/runtime.go`**: Remove static `openai.*` provider setup; bootstrap with `ProvidersConfigService` driven flow.
6. **`apps/sonal-ui`**: Model picker in Chat, models field in Providers CRUD.
7. **`tests/agent/integration-cli`**: Add `--providers-config` and `--model` flags; load config and seed `ProvidersConfigService`.

## 4. Detailed Architecture

### 4.1 ModelsLocator (`runtime/internal/models_locator.go`)

New internal component:

```go
type cachedGenkitInstance struct {
    genkit    *genkit.Genkit
    updatedAt time.Time
}

type ModelsLocator struct {
    mu             sync.Mutex
    providersSvc   ProvidersConfigService
    cache          map[string]*cachedGenkitInstance  // keyed by provider name, mutex-protected
    toolsRegistry  ToolsProvider  // for re-registering tool stubs
    toolStubRegistrar func(g *genkit.Genkit) // closure over ToolsRegistry.defineGenkitToolStubs
    logger         *slog.Logger
}
```

The `mu sync.Mutex` guards all cache reads and writes. Every `ResolveModel` call acquires the lock before inspecting or mutating the `cache` map. This prevents races when concurrent agent runs resolve models for the same (or different) providers simultaneously.

Key methods:
- `ResolveModel(ctx, fqModelName string) (model.LLM, error)` -- Parses `provider/model`, looks up provider config, checks cache freshness (`UpdatedAt`), creates new genkit instance if stale, returns `genkitLLMAdapter`. All cache access is mutex-protected.
- `ListModels(ctx) ([]ModelInfo, error)` -- Returns all configured models across all providers (each entry includes the provider name for fully qualified identification).

Cache invalidation: On each `ResolveModel` call, fetch provider config from `ProvidersConfigService.Get()`. Compare `UpdatedAt` with cached value. If different (or cache miss), create new `*genkit.Genkit` with that single provider's plugin, register tool stubs, and update cache. Old genkit instance is simply discarded (no explicit teardown needed -- genkit has no `Close()`).

**Tool stubs re-registration:** When a new genkit instance is created, call `toolStubRegistrar(newG)` which invokes `ToolsRegistry.defineGenkitToolStubs(newG)`. This is cheap (schema-only registration). ADK tools are unaffected (they don't depend on genkit).

### 4.2 ProviderConfig Model Extension (`runtime/internal/providers_config.go`)

Add a `Models` field to `ProviderConfig`:

```go
type ModelConfig struct {
    Name        string // e.g. "gpt-4.1"
    DisplayName string // e.g. "GPT 4.1"
}

type ProviderConfig struct {
    // ... existing fields ...
    Models []ModelConfig
}
```

Add `Models` to `CreateProviderConfigParams` and `UpdateProviderConfigParams`.

Persistence changes:
- **File-based:** Models stored as part of provider JSON file.
- **DB-based:** New `provider_models` table or JSON column on provider config table (JSON column preferred for simplicity -- GORM supports `datatypes.JSONType`).

### 4.3 Runner Refactoring (`runtime/agent/runner.go`)

Current `Runner` takes `[]Provider` at construction and calls `genkit.Init` once. This changes to:

- `RunnerArgs.Providers` removed.
- `Runner` holds a `ModelsLocator` instead of a static `*genkit.Genkit`.
- `LLMAdapterFactory` becomes a method on `ModelsLocator`.
- `newAgentRunnerParams` passes `ModelName` from `RunParams` (no longer hardcoded to `""`).

New `RunnerArgs`:

```go
type RunnerArgs struct {
    ProvidersConfigService ProvidersConfigService
}
```

The `NewRunner` constructor creates a `ModelsLocator` internally.

**Backward compatibility:** `RunnerArgs.Providers` can be kept temporarily during the implementation to avoid breaking all tests at once. After implementation is complete, `RunnerArgs.Providers` will be removed entirely -- backward compatibility is not a concern outside the project.

### 4.4 RunParams Model Field (`runtime/internal/agentrun.go`)

`RunParams` gains a `Model` field:

```go
type RunParams struct {
    UserID    string
    SessionID string
    Message   *MessageContent
    Model     string // fully qualified: "provider/model-name"
}
```

`AgentRunnerFactory.NewAgentRunner` already receives `ModelName` via `NewAgentRunnerParams.ModelName`. The chain becomes:
- API handler reads `request.Model` and sets `RunParams.Model`.
- `Runner.Run()` passes `RunParams.Model` into `newAgentRunnerParams().ModelName`.
- `AgentRunnerFactory.NewAgentRunner` calls `LLMAdapterFactory(modelName)` which goes through `ModelsLocator.ResolveModel`.

### 4.5 API Layer Changes (`runtime/internal/agentapi/`)

#### 4.5.1 Wire model field in agent runs
In `server.go`, `parseAgentRunRequest` already decodes `AgentRunRequest.Model`. Currently ignored. Change `StartAgentRun` and `ContinueAgentRun` to pass `request.Model` into `RunParams.Model`.

#### 4.5.2 Provider models endpoints
New endpoints in OpenAPI spec:

- `GET /models` -- List all models across all providers (returns provider name + model info for each entry).
- Update `CreateProviderRequest` and `UpdateProviderRequest` schemas to include `models` array.
- Update `ProviderResponse` schema to include `models` array.

#### 4.5.3 Models listing
New handler `ListModels` returns all configured models from all providers via `ProvidersConfigService`.

### 4.6 UI Changes (`apps/sonal-ui/`)

#### 4.6.1 Model picker in Chat
- Add a dropdown/select above the composer in `Chat.svelte`.
- On mount, fetch all providers + their models.
- User selects a model; the selection is passed as `model` field in `AgentRunRequest`.
- Default: first available model or last used model (localStorage).

#### 4.6.2 Models in Provider CRUD
- Extend provider create/edit forms with a models list (add/remove model entries).
- Each model entry: `name` (required), `displayName` (optional).

#### 4.6.3 New API client functions
- `listModels(params)` -- calls `GET /models` to fetch all models across all providers.
- Update `createProvider` / `updateProvider` to include models array.

### 4.7 Sonalmod App Changes (`apps/sonalmod/`)

#### 4.7.1 Remove static provider config
- Remove `openai.*` config keys from `default.yaml` and `provide.go`.
- Remove `OpenAIProvider`, `OpenAIDefaultModel`, `OpenAIBaseURL`, `OpenAIAPIKey` from `RuntimeDeps`.
- `newRuntime` no longer creates `agent.NewOpenAICompatibleLLMProvider` directly.
- Instead, `newRuntime` passes `ProvidersConfigService` into `agent.NewRunner` via `RunnerArgs`.

### 4.8 Integration CLI Changes (`tests/agent/integration-cli/`)

#### 4.8.1 New CLI flags
- `--providers-config` (required): Path to a JSON/YAML file containing provider configurations.
- `--model` (required): Fully qualified model name (e.g., `my-provider/my-model`).

#### 4.8.2 Config file format
```yaml
providers:
  - name: my-provider
    type: openai-compatible
    baseUrl: https://api.example.com/v1
    apiKey: sk-xxx
    models:
      - name: my-model
        displayName: My Model
```

#### 4.8.3 Config loading
On startup, read the config file, create a `FileProvidersConfigService` (in a temp dir), seed it with providers from the config file, and pass it to the engine/runner.

#### 4.8.4 Example config
Include `tests/agent/integration-cli/providers-config.example.yaml` in the repo. The actual config file must be gitignored.

#### 4.8.5 Agent test support
- Copy/adapt `local-user.yaml` to provide agent configuration for integration tests.
- Ensure the config file location is gitignored.
- The integration-cli `run` command uses `--providers-config` and `--model` to configure the runtime.

## 5. Key Architectural Decisions

1. **One genkit instance per provider (not global):** Each provider gets its own `*genkit.Genkit` instance. This is necessary because `genkit.Init` accepts plugins at creation time and cannot be extended later. Per-provider instances also provide clean isolation.

2. **Cache invalidation by `UpdatedAt` timestamp:** Simple, correct, and requires no pub/sub or event system. Each `ResolveModel` call does one `ProvidersConfigService.Get()` (fast for both file and DB backends).

3. **ModelsLocator is internal:** Not exposed in public contract. Embedders interact through `Runner` and `RunnerArgs`. This follows the project convention of keeping ADK/genkit internals unexported.

4. **Models are manually configured (not auto-discovered):** Auto-discovery from provider endpoints is a future enhancement. For now, models are explicitly listed in provider config. This is simpler and avoids provider-specific API differences.

5. **Static provider config removed from sonalmod:** Single source of truth for providers is `ProvidersConfigService`. No more parallel `openai.*` config pathway.

6. **Integration CLI uses config file, not embedded YAML:** Sensitive credentials stay out of `go:embed` and git. The config file is gitignored; only an example is committed.

## 6. Uncertainties

1. **Concurrent genkit.Init calls:** `genkit.Init()` may have global side effects (e.g., global registries). Need to verify it's safe to call multiple times with different plugins. If not, a global mutex around init may be needed.

2. **Tool stub re-registration:** Registering same tool names on multiple genkit instances should work (each instance has its own registry), but needs verification.

3. **Default model when none specified:** If `RunParams.Model` is empty, need a fallback strategy. Options: (a) use first provider's first model, (b) require model always, (c) configurable default in app config. Recommend (b) for clarity -- return 400 if no model specified and no default configured.

4. **Provider deletion while cached:** If a provider is deleted while a cached genkit instance exists, subsequent model resolution will fail with "provider not found." This is correct behavior. In-flight runs using the old instance will complete normally.

5. **Migration path:** Existing deployments have `openai.*` config. Developers will need to reconfigure providers via the UI/API after upgrading.

## 7. Related Files

### Files to modify

| File | Change |
|------|--------|
| `runtime/internal/providers_config.go` | Add `ModelConfig` struct, `Models` field to `ProviderConfig`, `CreateProviderConfigParams`, `UpdateProviderConfigParams` |
| `runtime/internal/file_providers_config_service.go` | Persist/load `Models` field |
| `runtime/internal/db_providers_config_service.go` | Persist/load `Models` field (JSON column or relation) |
| `runtime/internal/agentrun.go` | Add `Model` field to `RunParams` |
| `runtime/internal/genkit_adapter.go` | Possibly minor adjustments for per-provider adapter creation |
| `runtime/agent/runner.go` | Major refactor: use `ModelsLocator`, accept `ProvidersConfigService`, remove static `Providers` |
| `runtime/agent/provider.go` | May become simpler or internal-only |
| `runtime/agent/providers_config.go` | Re-export new types |
| `runtime/agent/tools_registry.go` | Expose `defineGenkitToolStubs` as callable from `ModelsLocator` |
| `runtime/internal/agentapi/openapi.yaml` | Add models to provider schemas, add `GET /models` endpoint, document model field usage |
| `runtime/internal/agentapi/server.go` | Wire `request.Model` into `RunParams.Model` |
| `runtime/internal/agentapi/provider_handlers.go` | Add `ListModels` handler |
| `runtime/internal/agentapi/provider_mapper.go` | Map `Models` to/from API |
| `runtime/internal/agentapi/api.gen.go` | Regenerated from spec |
| `runtime/httpapi/handler.go` | Pass `ModelsLocator` or `ProvidersConfigService` |
| `apps/sonalmod/internal/runtime.go` | Remove static provider setup, pass `ProvidersConfigService` to runner |
| `apps/sonalmod/internal/config/default.yaml` | Remove `openai.*` keys (or deprecate) |
| `apps/sonalmod/internal/config/provide.go` | Remove openai config bindings |
| `apps/sonal-ui/src/pages/Chat.svelte` | Add model picker |
| `apps/sonal-ui/src/pages/Providers.svelte` | Add models list to provider forms |
| `apps/sonal-ui/src/lib/agentapi/client.ts` | Add `listProviderModels` function |
| `apps/sonal-ui/src/lib/agentapi/types.ts` | Re-export new types |
| `apps/sonal-ui/ui-wireframe.md` | Update with model picker and provider models UI |
| `tests/agent/integration-cli/main.go` | Add `--providers-config` and `--model` flags |
| `tests/agent/integration-cli/cli.go` | Use model parameter in agent run |

### Files to create

| File | Purpose |
|------|---------|
| `runtime/internal/models_locator.go` | `ModelsLocator` implementation |
| `runtime/internal/models_locator_test.go` | Tests for `ModelsLocator` |
| `tests/agent/integration-cli/providers-config.example.yaml` | Example providers config file |
| `doc/implementation/dynamic-providers-and-model-selection/plan-dynamic-providers-and-model-selection.md` | This plan |

### Files to verify / update gitignore

| File | Purpose |
|------|---------|
| `tests/agent/integration-cli/.gitignore` | Add providers config pattern |
| `.gitignore` | Verify `*-user.yaml` pattern coverage |

## 8. Task List

Implementation follows TDD approach. Each task is self-contained and leaves the codebase in a buildable state per module-specific task completion protocol.

---

### Phase 1: Runtime Core -- ModelsLocator and Provider Models

**Task 1.1: Extend ProviderConfig with Models field**
- Add `ModelConfig` struct to `runtime/internal/providers_config.go`
- Add `Models []ModelConfig` to `ProviderConfig`, `CreateProviderConfigParams`, `UpdateProviderConfigParams`
- Write failing tests for file-based service:
  - Creating provider with models persists and returns models
  - Updating provider models persists and returns updated models
  - Provider with no models returns empty slice (not nil)
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation (fields missing, not compilation errors)
- Implement file-based persistence for `Models` field in `file_providers_config_service.go`
- Write failing tests for DB-based service (same cases as file-based)
- Implement DB-based persistence for `Models` field in `db_providers_config_service.go`
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-1.1.md`
- All checks from completion protocol must be passed

**Task 1.2: Add Model field to RunParams**
- Add `Model string` field to `RunParams` in `runtime/internal/agentrun.go`
- Update `Runner.Run()` in `runtime/agent/runner.go` to pass `RunParams.Model` to `newAgentRunnerParams().ModelName`
- Update `Runner.newAgentRunnerParams()` to accept model name parameter
- Write failing tests:
  - `AgentRunnerFactory.NewAgentRunner` receives non-empty `ModelName` and passes it to `LLMAdapterFactory`
  - `Runner.Run` with `Model` set in `RunParams` propagates to adapter factory
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation
- Implement the model propagation
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-1.2.md`
- All checks from completion protocol must be passed

**Task 1.3: Implement ModelsLocator**
- Create `runtime/internal/models_locator.go`
- Implement `ModelsLocator` struct with:
  - `NewModelsLocator(params)` constructor
  - `ResolveModel(ctx, fqModelName) (model.LLM, error)` -- parse provider/model, lookup provider config, cache management, genkit init, tool stub registration
  - `ListModels(ctx) ([]ModelInfo, error)` -- list all models across all providers
- Write failing tests in `runtime/internal/models_locator_test.go`:
  - Resolve model from provider that exists -> returns LLM adapter
  - Resolve model from unknown provider -> error
  - Resolve model after provider update (different `UpdatedAt`) -> creates new genkit instance
  - Resolve model with same `UpdatedAt` -> reuses cached genkit instance
  - Concurrent resolve calls for same provider -> only one genkit.Init (mutex protects cache)
  - Tool stubs registered on each new genkit instance
  - ListModels returns models from all providers
  - ListModels with no providers -> empty slice
  - Parse model name: "provider/model" -> correct provider and model
  - Parse model name without "/" -> error
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation (stubs should exist, not compilation errors)
- Implement `ModelsLocator` logic
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-1.3.md`
- All checks from completion protocol must be passed

**Task 1.4: Refactor Runner to use ModelsLocator**
- Update `RunnerArgs`: replace `Providers` with `ProvidersConfigService` (keep `Providers` temporarily during implementation to avoid breaking all tests at once; remove entirely after implementation is complete)
- Update `NewRunner` to create `ModelsLocator` internally when `ProvidersConfigService` is provided
- Update `LLMAdapterFactory` to delegate to `ModelsLocator.ResolveModel` when available
- Ensure `ToolsRegistry.defineGenkitToolStubs` is accessible to `ModelsLocator` (may need to expose via closure or interface)
- Write failing tests:
  - `NewRunner` with `ProvidersConfigService` creates runner successfully
  - Runner.Run with `Model` set resolves through `ModelsLocator`
  - Runner.Run with `Model` set to unknown provider returns error
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation
- Implement runner refactoring
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-1.4.md`
- All checks from completion protocol must be passed

---

### Phase 2: API Layer

**Task 2.1: Wire model field in agent run API**
- In `runtime/internal/agentapi/server.go`, update `parseAgentRunRequest` to return model string from `AgentRunRequest.Model`
- Update `StartAgentRun` and `ContinueAgentRun` to pass model into `RunParams.Model`
- Write failing tests:
  - StartAgentRun with model field → RunParams.Model set correctly
  - StartAgentRun without model field → RunParams.Model is empty
  - ContinueAgentRun with model field → RunParams.Model set correctly
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation
- Implement the wiring
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-2.1.md`
- All checks from completion protocol must be passed

**Task 2.2: Add models to provider API schemas**
- Update `openapi.yaml`:
  - Add `ModelConfig` schema: `{ name: string, displayName?: string }`
  - Add `models` array to `CreateProviderRequest`, `UpdateProviderRequest`, `ProviderResponse`
  - Add `GET /models` endpoint returning `{ models: [{ provider: string, name: string, displayName?: string }] }` -- lists all models across all providers
- Regenerate `api.gen.go`: `go generate ./internal/agentapi`
- Update `provider_mapper.go` to map `Models` field
- Update `provider_handlers.go`:
  - Existing handlers pass `Models` through
  - Add `ListModels` handler (delegates to `ModelsLocator.ListModels`)
- Write failing tests:
  - Create provider with models -> response includes models
  - Update provider models -> response reflects update
  - GET provider -> response includes models
  - List all models -> returns models from all providers with provider name
  - List all models with no providers -> empty array
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify failure is expectation
- Implement handler + mapper changes
- Run affected tests: `npx nx test runtime --skip-nx-cache`
  - Verify all tests pass
- Regenerate UI types: `make generate-api` from `apps/sonal-ui`
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-2.2.md`
- All checks from completion protocol must be passed

---

### Phase 3: Sonalmod App Changes

**Task 3.1: Remove static provider config from sonalmod**
- Remove `openai.provider`, `openai.defaultModel`, `openai.baseURL`, `openai.apiKey` from `default.yaml`
- Remove corresponding config bindings from `provide.go`
- Remove `OpenAIProvider`, `OpenAIDefaultModel`, `OpenAIBaseURL`, `OpenAIAPIKey` from `RuntimeDeps`
- Update `newRuntime` to:
  - Create `ProvidersConfigService` first (already done, mostly)
  - Pass `ProvidersConfigService` to `agent.NewRunner` via `RunnerArgs` (instead of `Providers` slice)
  - Remove `agent.NewOpenAICompatibleLLMProvider` call
- Update `Engine` if needed (accessor changes)
- Write failing tests (if runtime.go has tests):
  - Verify runtime creates successfully with only `ProvidersConfigService`
  - Verify no panics on startup without `openai.*` config
- Run affected tests: `npx nx test sonalmod --skip-nx-cache`
  - Verify all tests pass
- Run `make affected-lint-test` from repo root
  - Verify all pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-3.1.md`
- All checks from completion protocol must be passed

---

### Phase 4: UI Changes

**Task 4.1: Add models to Provider CRUD forms**
- Update `Providers.svelte` create/edit forms to include a models list:
  - Each model entry: `name` (text input, required), `displayName` (text input, optional)
  - Add/remove model entries dynamically
- Update API client calls to include `models` in create/update payloads
- Update provider display to show model count or list
- Write tests:
  - Create provider form shows models inputs
  - Add model entry → form shows new entry
  - Submit create with models → API called with models
  - Edit provider shows existing models
- Run tests: `npx nx test sonal-ui --skip-nx-cache`
  - Verify all tests pass
- Update `ui-wireframe.md` with models in provider forms
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-4.1.md`
- All checks from completion protocol must be passed

**Task 4.2: Add model picker to Chat page**
- Add model picker dropdown above composer in `Chat.svelte`
- On mount, fetch all models via `GET /models` (single call, all providers)
- Add `listModels` client function -- calls `GET /models`
- Selected model stored in localStorage for persistence across page loads
- Pass selected model as `model` field in `AgentRunRequest` body
- Write tests:
  - Model picker renders with available models
  - Selecting model updates state
  - Submit sends model in request body
  - Default selection from localStorage
- Run tests: `npx nx test sonal-ui --skip-nx-cache`
  - Verify all tests pass
- Update `ui-wireframe.md` with model picker
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-4.2.md`
- All checks from completion protocol must be passed

---

### Phase 5: Integration Tests

**Task 5.1: Add --providers-config and --model to integration-cli**
- Add `--providers-config` flag (required for `run` command): path to YAML file with provider configs
- Add `--model` flag (required for `run` command): fully qualified model name
- Create `providers-config.example.yaml` with example structure
- Update `.gitignore` to exclude actual config files (e.g., `providers-config.yaml`, `providers-config.local.yaml`)
- Load config file on startup:
  - Parse YAML into provider config structs
  - Create temp `FileProvidersConfigService`
  - Seed with providers from config file
  - Pass to engine/runner
- Pass `--model` value into `cliParams` and through to `runCLI` → `RunParams.Model`
- Write failing unit tests:
  - Parse providers config YAML file correctly
  - Missing config file → error
  - Invalid YAML → error
  - Model flag passed into RunParams
  - Missing model flag → error
- Run tests: `npx nx test integration-cli --skip-nx-cache`
  - Verify failure is expectation
- Implement config loading and model passing
- Run tests: `npx nx test integration-cli --skip-nx-cache`
  - Verify all tests pass
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-5.1.md`
- All checks from completion protocol must be passed

**Task 5.2: Enable agent integration tests with provider config**
- Update agent test scenarios (`tests/agent/scenarios/`) if needed to pass model
- Ensure `integration-cli run` works with `--providers-config` and `--model` flags
- Create a gitignored config location for agent test configs (e.g., `tests/agent/integration-cli/providers-config.yaml`)
- Verify `.gitignore` patterns exclude sensitive config
- Document how to set up and run agent tests with real providers in `tests/AGENTS.md`
- Copy/adapt `local-user.yaml` pattern for integration test configs
- Manual verification: run `integration-cli run --providers-config ./providers-config.yaml --model provider/model --prompt "hello"` with real provider
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-5.2.md`
- All checks from completion protocol must be passed

---

### Phase 6: Final Validation

**Task 6.1: End-to-end validation and cleanup**
- Run full `make affected-lint-test` from repo root -- all must pass
- Verify UI codegen is in sync: `make check-api` from `apps/sonal-ui`
- Review and clean up any TODO comments left during implementation
- Verify no sensitive data in committed files (grep for API keys, passwords)
- Update `runtime/AGENTS.md` if public contract changed
- Update `apps/sonalmod/AGENTS.md` if config/commands changed
- Update `tests/AGENTS.md` with new test run instructions
- Write summary to `doc/implementation/dynamic-providers-and-model-selection/summary-task-6.1.md`
- All checks from completion protocol must be passed

**Task 6.2: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
