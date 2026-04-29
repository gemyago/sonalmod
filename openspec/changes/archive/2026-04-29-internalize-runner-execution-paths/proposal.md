## Why

`runtime/agent/runner.go` still owns too much execution-path logic for direct runs, regular profiles, and `acp-stdio` profiles. That makes the public runner harder to reason about, easier to bloat, and inconsistent with the runtime rule that public contract layers should stay minimal while internal packages own mode-specific behavior.

## What Changes

- Move execution-path branching and mode-specific run preparation out of the public `agent.Runner` implementation into an internal runner component.
- Keep the public `agent.Runner` as a thin orchestrator over validation, dependency wiring, and delegation to internal execution helpers.
- Centralize loading of profile-backed execution settings and resolution of direct, regular-profile, and `acp-stdio` execution paths behind an internal boundary.
- Preserve existing external run behavior, including request-level model rules for direct and regular profile runs and ignored request-level model behavior for `acp-stdio` profiles.
- Tighten tests and docs around the public-vs-internal execution ownership boundary so future refactors do not reintroduce mode-specific logic into the public layer.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: execution-path ownership is tightened so public runner APIs remain thin orchestrators while internal runtime code owns execution-path selection and mode-specific run setup.

## Impact

- `runtime/agent/runner.go` and supporting runner tests.
- Internal runtime execution wiring for direct runs, regular profiles, and `acp-stdio` profiles.
- Runtime docs that describe runner responsibilities and public contract boundaries.
