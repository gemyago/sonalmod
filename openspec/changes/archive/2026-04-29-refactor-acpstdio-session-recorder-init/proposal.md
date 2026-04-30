## Why

ACP stdio profile execution setup is currently split between `agent.Runner` and ACP-specific internals, which makes ownership unclear and increases constructor coupling in runner wiring. Moving session recorder initialization into the ACP profile runner and switching to a params-struct input aligns this code with the established runtime pattern of consumer-defined interfaces and struct-based request objects.

## What Changes

- Refactor ACP stdio profile runner construction so it owns `acpstdio.SessionRecorder` initialization.
- Replace argument lists used by ACP profile execution entry points with a dedicated request params struct.
- Simplify `agent.Runner` ACP setup by delegating recorder/profile-runner construction to ACP-owned internals.
- Preserve existing standard run behavior and API semantics for `acp-stdio` profiles.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `agent-profile-execution-settings`: ACP stdio profile execution wiring is tightened so ACP-specific internals own recorder/runtime construction while preserving the same externally observable run behavior.

## Impact

- `runtime/agent/runner.go` ACP profile runner initialization path.
- ACP stdio internal execution package (constructor shape and dependency wiring).
- Runtime tests covering ACP profile execution wiring and unchanged run semantics.
