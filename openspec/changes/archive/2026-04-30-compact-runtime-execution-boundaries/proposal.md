## Why

`runtime/internal/profileexec.go` currently owns run-path dispatch even though the built-in execution path already lives in `runtime/internal/agentrun.go`, which makes the runtime harder to follow and keeps ACP stdio logic split across multiple internal packages. The current error flow also risks leaking lower-level dispatch details through the standard HTTP contract instead of presenting a small, stable public error surface.

## What Changes

- Decommission `runtime/internal/profileexec.go` as a separate dispatch owner and move direct-run and regular-profile orchestration into `runtime/internal/agentrun.go`.
- Merge `runtime/internal/codinglane` into `runtime/internal/acpstdio` so ACP stdio request mapping, subprocess execution, result translation, and session recording live behind one ACP-focused internal boundary.
- Tighten standard run error handling so profile-selection and execution-mode failures keep stable classification without exposing incidental low-level error details through public problem responses.
- Preserve current execution semantics: direct runs still require a request model, regular profiles still resolve request-model override versus profile default, and `acp-stdio` profiles still ignore request-level model overrides.
- Update runtime tests and docs to describe the compacted ownership boundaries instead of the removed wrapper/package split.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: clarifies that built-in dispatch is owned by the standard agent-run path, ACP stdio internals are consolidated behind one ACP boundary, and standard run profile-selection failures use a stable public error contract.

## Impact

- `runtime/internal/agentrun.go` and related runner wiring in `runtime/agent/runner.go`
- `runtime/internal/profileexec.go` removal and test migration
- `runtime/internal/acpstdio/` and `runtime/internal/codinglane/` consolidation
- `runtime/internal/agentexec_error.go` and `runtime/internal/agentapi/server.go` error handling behavior
- Runtime docs and tests that currently describe profile dispatch or ACP execution boundaries
