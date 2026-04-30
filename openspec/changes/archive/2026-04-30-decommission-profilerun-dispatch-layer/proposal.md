## Why

`runtime/internal/profilerun` currently acts as a generic dispatch layer that resolves profiles and then immediately delegates regular runs back into the standard built-in runner path. That extra layer makes runtime execution harder to understand, duplicates orchestration responsibilities, and obscures the fact that only `acp-stdio` execution actually needs specialized behavior.

## What Changes

- Decommission `runtime/internal/profilerun` as the generic profile execution package.
- Move direct-run and regular-profile dispatch into the standard internal agent run path so one runtime owner decides how built-in execution happens.
- Keep only ACP-specific execution, result mapping, and session recording in `runtime/internal/acpstdio`.
- Preserve current profile execution semantics: direct runs still require a request model, regular profiles still resolve model override vs profile default, and `acp-stdio` profiles still ignore request-level model overrides.
- Preserve the existing error classification expected by the HTTP API when profile selection is invalid, missing, unsupported, or fails at execution time.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: standard run dispatch ownership changes so regular profile execution is part of the standard internal agent run path, while only ACP-specific execution remains in ACP internals.

## Impact

- `runtime/internal/profilerun/` removal and migration of any surviving behavior.
- `runtime/internal/agentrun.go` and related internal runtime execution wiring for direct and regular-profile runs.
- `runtime/internal/acpstdio/` boundaries for ACP-specific execution and session recording.
- `runtime/internal/agentapi/` error handling that maps profile execution failures into HTTP responses.
- Runtime docs and tests that currently describe or exercise `profilerun` as the profile dispatch owner.
