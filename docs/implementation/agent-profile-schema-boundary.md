# Agent Profile Schema Boundary

This document defines the persisted agent profile contract after profile-based
execution settings replaced the old OpenCode-specific public lane. It records
what is part of general `Agent` profile data now, and what remains deferred as
backend-specific `Connection` data.

References:
- `docs/domain-terminology.md` (`Agent` vs `Connection`)
- `.planning/phases/02-agent-profile-foundation/02-CONTEXT.md`
- `docs/implementation/opencode-acp-capability-map.md`

## General Profile Data

Persisted general profile data includes:

- `name` (immutable identifier, unique key)
- `displayName` (mutable label)
- `role` (mutable role/persona intent)
- `instructions` (mutable instructions)
- `toolRefs` (mutable list of selected tool identifiers)
- `executionSettings` (mutable runtime-owned execution contract)
- `createdAt`, `updatedAt` (system-managed timestamps)

Identifier strategy:
- `name` is immutable after create and is the canonical lookup key.
- Updates modify mutable fields only (`displayName`, `role`, `instructions`,
  `toolRefs`, `executionSettings`).

Persistence locations in Phase 2:
- File mode (`agentRuntime.storage.type=file`): `{dataDir}/agent-profiles/{name}.yaml`
- Database mode (`agentRuntime.storage.type=database`): `agent_profiles` table
  using the configured runtime table prefix (`agentRuntime.database.tablePrefix`).

Runtime CRUD endpoints:
- `GET /api/v1/runtime/agent-profiles`
- `GET /api/v1/runtime/agent-profiles/{name}`
- `POST /api/v1/runtime/agent-profiles`
- `PATCH /api/v1/runtime/agent-profiles/{name}`
- `DELETE /api/v1/runtime/agent-profiles/{name}`

### Execution settings shapes

`executionSettings` now owns both built-in and ACP stdio execution defaults:

- `regular` mode (or omitted mode): requires `defaultModel`
- `acp-stdio` mode: requires `agentCommand.command`, `agentCommand.args`, and
  may include `cwd`

This keeps standard run selection profile-centric: runtime run requests identify
`profileName`, and the selected profile decides whether execution is built-in or
ACP stdio.

## Deferred Connection Or Backend Data

The following remain outside the general profile schema and belong to
backend-specific connection/configuration work:

- MCP server injection payloads not represented by `executionSettings`
- ACP capability flags and negotiated protocol/runtime capabilities
- Remote runtime session identifiers and resume/load handles
- Slash-command inventories and backend-emitted command catalogs
- Backend/provider-specific launch options, transport settings, or execution
  flags not owned by the general Sonalmod profile contract

This follows the glossary boundary:
- `Agent`: reusable specialist definition (general profile shape above)
- `Connection`: link to a concrete runtime/backend and its operational details

## Boundary Table

| Field / Concept | General Profile Data | Deferred Connection Or Backend Data |
| --- | --- | --- |
| Identifier | `name` | Remote runtime IDs |
| Presentation | `displayName` | Backend-specific labels/aliases |
| Behavior intent | `role`, `instructions` | Backend-specific launch flags |
| Tool selection | `toolRefs` | Slash-command inventories |
| Runtime defaults | `executionSettings` | Backend transport/options outside the schema |
| Working directory | `executionSettings.cwd` for `acp-stdio` | Other backend-local paths or handles |
| MCP injection | No | `mcpServers` |
| Capability semantics | No | ACP capability flags |
| Session linkage | No | Remote session identifiers |
