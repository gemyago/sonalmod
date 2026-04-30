## Why

The current profile execution implementation adds a generic `profileexec.Dispatcher` layer that immediately calls back into `agent.Runner` for regular profiles, making the execution path harder to reason about without adding a useful abstraction boundary. It also treats `AgentProfilesService` as optional even though saved profile execution is now part of the standard runner contract.

## What Changes

- Make `RunnerArgs.AgentProfilesService` required for `agent.NewRunner`; constructing a runtime runner without a profiles service returns a clear configuration error.
- Move regular profile resolution and execution directly into `agent.Runner.Run`.
- Remove the generic profile dispatcher/wrapper path for regular profiles.
- Keep the existing direct built-in execution path for requests without `profileName`.
- When `Runner.Run` resolves a profile whose mode is `regular` or omitted, execute through the built-in runner using the existing model precedence and profile instruction behavior.
- When `Runner.Run` resolves a profile whose mode is `acp-stdio`, delegate only the ACP-specific execution, result mapping, and session recording to ACP-owned internal logic.
- Refactor `runtime/internal/profileexec` so any surviving code is ACP-specific or moved to a more accurate ACP-focused package/name.
- Update runtime docs/tests so the runner, not a generic dispatcher, is the owner of profile-backed standard runs.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: profile service availability and standard run dispatch ownership are tightened so profile-backed execution is always runner-owned, with only ACP stdio execution delegated to ACP-specific internals.

## Impact

- `runtime/agent/runner.go` and runner tests for constructor validation, profile lookup, regular profile execution, and ACP delegation.
- `runtime/internal/profileexec` and related tests, likely renamed or reduced to ACP-specific execution helpers.
- `runtime/internal/codinglane` ACP stdio executor/result/session recording boundaries as needed.
- Runtime public contract docs in `runtime/AGENTS.md` and any tests or wiring that construct `agent.NewRunner`.
- Bundled backend and high-level test fixtures that currently pass runner args without an agent profiles service.
