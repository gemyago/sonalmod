## Important Apply Phase Constraint

Applying phase must follow [orchestrate-tasks](../../orchestrate-tasks.md) instruction.

---

## 1. Profile Execution Settings Contract

- [x] 1.1 Extend the runtime agent profile domain with `regular` and `acp-stdio` execution setting shapes, treating omitted mode as `regular`, and add unit tests for accepted and rejected settings.
- [x] 1.2 Update file-backed profile persistence to round-trip omitted regular mode, explicit regular mode, and ACP stdio settings without losing existing regular profiles.
- [x] 1.3 Update database-backed profile persistence and migrations to store the new execution settings shape, with tests for existing regular records and ACP stdio records.
- [x] 1.4 Update profile API mapping and validation so profile CRUD accepts regular settings with `defaultModel`, accepts ACP stdio settings without `defaultModel`, and rejects unsupported modes or invalid command settings.

## 2. Public Profile API Contract

- [x] 2.1 Update `runtime/internal/agentapi/openapi.yaml` profile schemas for mode-specific execution settings, regenerate Go API code, and update runtime profile handler tests.
- [x] 2.2 Regenerate the Svelte OpenAPI client after the profile schema change and update any affected frontend type fixtures or tests.
- [x] 2.3 Add API tests proving existing profiles without `executionSettings.mode` are returned as valid regular profiles and ACP stdio profiles are returned with command, args, and optional `cwd`.

## 3. Profile-Aware Standard Runs

- [x] 3.1 Introduce a profile execution dispatcher below the HTTP handler that loads profiles and routes regular profiles to the built-in runner using `executionSettings.defaultModel`.
- [x] 3.2 Teach standard start-run and continue-run paths to accept `profileName` while preserving the current request model path until the API removal task, and add handler tests for regular profile dispatch.
- [x] 3.3 Add not-found and validation handling for missing, unknown, or unusable `profileName` values using the standard problem/error responses.
- [x] 3.4 Update bundled backend wiring in `apps/sonalmod` so the runtime HTTP handler receives the profile-aware execution dependencies without exposing ACP process details.

## 4. ACP Stdio Execution Path

- [x] 4.1 Extract or adapt the existing ACP process code into a generic internal ACP stdio executor that consumes profile command settings instead of OpenCode binding records.
- [x] 4.2 Wire ACP stdio profiles through the profile execution dispatcher and standard SSE stream contract, including `sessionBound`, `agent` or `error`, and `done` events.
- [x] 4.3 Persist ACP stdio run output through the same session storage path used by regular runs so `GET /sessions/{sessionId}` can replay completed ACP stdio history.
- [x] 4.4 Add tests for ACP stdio launch success, launch failure, protocol failure, and session read-back behavior through standard run and session endpoints.

## 5. Remove Request-Level Model Selection

- [x] 5.1 Change standard run OpenAPI request bodies to require `profileName` and remove request-level `model`, then regenerate Go API code and update request mapper tests.
- [x] 5.2 Update Svelte generated API types, chat client calls, fixtures, and tests so standard runs send `profileName` instead of `model`.
- [x] 5.3 Remove regular-run compatibility code that accepted request-level `model` and add regression tests that missing `profileName` is rejected.

## 6. Remove OpenCode Public Surface

- [x] 6.1 Remove `/opencode-bindings`, `/opencode-bindings/{bindingName}`, and `/opencode-launches` from the OpenAPI contract, regenerate Go and Svelte API artifacts, and update affected tests.
- [x] 6.2 Delete OpenCode binding and launch handlers, mappers, generated-operation tests, persistence services, and bundled backend wiring that are no longer reachable.
- [x] 6.3 Remove exported OpenCode aliases, launcher constructors, and binding services from `runtime/agent` and remove OpenCode dependencies from `runtime/httpapi.HandlerArgs`.
- [x] 6.4 Rename surviving generic internals from OpenCode-specific names to ACP stdio/profile execution names, keeping OpenCode wording only in executable-specific leaf adapters, fixtures, or historical docs.

## 7. Verification and Documentation

- [x] 7.1 Update runtime and app AGENTS or module docs if public wiring, commands, or architecture notes changed during implementation.
- [x] 7.2 Run `make affected-lint-test` from the repository root and fix all lint, test, generation, or API drift failures.
- [x] 7.3 Verify the final OpenAPI and generated clients expose profile execution settings and standard profile-based runs, with no OpenCode public endpoints or exported runtime symbols.
