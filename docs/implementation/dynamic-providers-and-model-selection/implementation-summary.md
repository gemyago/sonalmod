# Implementation Summary: Dynamic Providers and Model Selection

**Plan:** [plan-dynamic-providers-and-model-selection.md](./plan-dynamic-providers-and-model-selection.md)

## Overview

Providers are fully dynamic: `ModelsLocator` caches a genkit instance per provider (invalidated by `UpdatedAt`), tool stubs re-register on new instances, and agent runs use a fully qualified `provider/model` from `RunParams` and the HTTP API. Provider config includes a `models` list persisted in file and DB backends; the OpenAPI layer exposes `models` on provider CRUD and `GET /models`. Sonalmod no longer wires static `openai.*` keys; the UI supports provider model editing and a Chat model picker; the integration CLI seeds `FileProvidersConfigService` from YAML and requires `--model`. Documentation and validation (`make affected-lint-test`, API check) were completed across the stack.

## Tasks

### Task 1.1: Extend ProviderConfig with Models field

Extended `ProviderConfig` (and create/update params) with `ModelConfig` / `Models`, persisted in file storage via JSON and in the DB with GORM JSON serialization, with tests for create/update and empty (non-nil) slices.

### Task 1.2: Add Model field to RunParams

Added `Model` on `RunParams` and wired `Runner.Run` through `newAgentRunnerParams` so the value reaches `LLMAdapterFactory` as `ModelName`, with tests in `agentrun_test.go` and `runner_test.go`.

### Task 1.3: Implement ModelsLocator

Added `ModelsLocator` with `ModelInfo`, mutex-backed cache, `ResolveModel` / `ListModels`, injectable `GenkitInitFunc`, and a `ToolStubRegistrar` hook; tests cover parsing, cache behavior, concurrency, and list cases.

### Task 1.4: Refactor Runner to use ModelsLocator

Runner accepts `ProvidersConfigService`, builds `ModelsLocator` (with optional test-only genkit init override), keeps the legacy `Providers` path, and resolves the model eagerly in `Run` before the agent loop; a `FakeGenkitInstance` helper supports tests.

### Task 2.1: Wire model field in agent run API

The agent run API parses optional `model` from `AgentRunRequest`, passes it through `StartAgentRun` and `ContinueAgentRun` into `RunParams.Model`, with tests asserting the wiring.

### Task 2.2: Add models to provider API schemas

OpenAPI and generated code were extended with model schemas, `models` on provider create/update/response, and `GET /models`; handlers, mappers, server wiring, mocks, and UI types were updated.

### Task 3.1: Remove static provider config from sonalmod

Removed static `openai.*` config and related `RuntimeDeps` wiring; runtime builds `ProvidersConfigService` first, passes it into `agent.NewRunner`, and connects listing to GET `/models` via the runner.

### Task 4.1: Add models to Provider CRUD forms

Provider create/edit forms include a dynamic Models section (name/displayName, add/remove), payloads include `models`, and the provider table shows a per-row model count; tests and wireframe docs were updated.

### Task 4.2: Add model picker to Chat page

Chat loads models via `listModels()`, shows a model `<select>` when models exist, persists the choice in `localStorage` under `selectedModel`, and sends the fully qualified name as `model` in `AgentRunRequest`.

### Task 5.1: Add --providers-config and --model to integration-cli

Required `--providers-config` and `--model` on the `run` subcommand load YAML into `FileProvidersConfigService` and wire the runner; tests and an example config were added.

### Task 5.2: Enable agent integration tests with provider config

`tests/AGENTS.md` and the master scenario document how to run the integration CLI with `--providers-config` and `--model`; `.gitignore` patterns for local configs were confirmed.

### Task 6.1: End-to-end validation and cleanup

Full affected lint/test and API codegen checks passed; scans found no stray TODOs or committed secrets; `runtime/AGENTS.md` documents the provider/model contract.

## Deviations & notes

| Area | Note |
|------|------|
| DB persistence (1.1) | Plan mentioned `datatypes.JSONType`; project used GORM `serializer:json` instead of adding `datatypes`. |
| ModelsLocator (1.3) | `ToolStubRegistrar` on params rather than only a closure on `ToolsRegistry`; full wiring completed in Task 1.4. |
| Runner (1.4) | Model resolution runs eagerly in `Run` (and hits cache in the factory) rather than only delegating via `LLMAdapterFactory`; `defineGenkitToolStubs` passed via closure at `NewRunner`. |
| API / httpapi (2.2) | `ModelsLister` placement and `ListModels` returning 200 with empty list when no lister is configured. |
| Sonalmod (3.1) | `ModelsLocator()` exposed as `agent.ModelsLister` / re-exports for embedder-safe boundaries; `TestNewRuntime` updated instead of wholly new tests. |
| Integration CLI (5.1) | Re-exports from `runtime/agent` for create params; runner built with `ProvidersConfigService` while engine still used for logging/config/tools. |
| Task 5.2 | Manual run against a real provider was not executed (needs live credentials); docs describe the step. |
| Task 6.1 | Legacy `RunnerArgs.Providers` kept for tests with a defer-removal comment; `check-api` may run via sonal-ui lint rather than a standalone `make check-api` in some flows. |

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
