## Why

OpenCode is currently exposed as its own public API lane and as exported runtime dependencies, which leaks a specific external agent implementation into the product contract. The runtime should instead expose agent profiles as the single public selector, with each profile's execution settings deciding whether the profile runs through the built-in agent runner or an ACP stdio agent.

## What Changes

- Replace OpenCode-specific public concepts with profile-scoped execution settings.
- Extend agent profile execution settings with an optional execution mode; omitted mode defaults to built-in regular execution and `acp-stdio` selects an external ACP stdio agent.
- Make standard agent run requests select a profile; regular model selection comes from that profile's execution settings.
- Store external ACP stdio command, args, and working directory inside the profile execution settings instead of separate OpenCode bindings.
- Route standard agent run endpoints through the selected profile's execution settings.
- Rename surviving execution internals to generic ACP stdio/profile execution concepts, keeping OpenCode names only in leaf command-specific code when unavoidable.
- Remove OpenCode binding and launch endpoints from the generated HTTP API.
- Remove exported OpenCode aliases, service constructors, and launcher dependencies from the public `runtime/agent` and `runtime/httpapi` surfaces.
- **BREAKING**: Clients using `/opencode-bindings`, `/opencode-launches`, or exported OpenCode runtime symbols must migrate to agent profiles plus standard session run endpoints.

## Capabilities

### New Capabilities

- `agent-profile-execution-settings`: Defines the profile execution-settings contract, including default regular built-in execution and ACP stdio execution.

### Modified Capabilities

None. There are no existing committed OpenSpec capability specs to modify.

## Impact

- Runtime profile domain and persistence in `runtime/internal/agentprofiles`.
- Runtime HTTP contract in `runtime/internal/agentapi/openapi.yaml` and generated API code.
- Runtime public package boundaries in `runtime/agent` and `runtime/httpapi`.
- Standard run handlers and request mapping in `runtime/internal/agentapi`.
- ACP stdio execution internals, including code currently named around OpenCode under `runtime/internal/codinglane`.
- Bundled backend wiring in `apps/sonalmod/internal/runtime.go`.
- Generated Svelte API client if it is present when the API contract is regenerated.
- This supersedes the older `add-custom-agent-backend` proposal shape: implementation should use `executionSettings`, not a parallel `customBackend` field.
