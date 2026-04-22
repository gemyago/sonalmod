# Agent Profile Schema Boundary (Phase 2)

This document defines the durable Phase 2 contract for persisted agent profiles.
It records what is part of general `Agent` profile data now, and what remains
deferred as backend-specific `Connection` data for later phases.

References:
- `docs/domain-terminology.md` (`Agent` vs `Connection`)
- `.planning/phases/02-agent-profile-foundation/02-CONTEXT.md`
- `docs/implementation/opencode-acp-capability-map.md`

## General Profile Data

Phase 2 persists only backend-agnostic profile data:

- `name` (immutable identifier, unique key)
- `displayName` (mutable label)
- `role` (mutable role/persona intent)
- `instructions` (mutable instructions)
- `toolRefs` (mutable list of selected tool identifiers)
- `executionSettings` (mutable runtime-owned defaults, currently `defaultModel`)
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

## Deferred Connection Or Backend Data

The following remain outside the Phase 2 general profile schema and belong to
backend-specific binding/configuration work:

- OpenCode/ACP runtime launch data such as `cwd` and `mcpServers`
- ACP capability flags and negotiated protocol/runtime capabilities
- Remote runtime session identifiers and resume/load handles
- Slash-command inventories and backend-emitted command catalogs
- Backend/provider-specific launch options, transport settings, or execution
  flags not owned by the general Sonalmod profile contract

This follows the glossary boundary:
- `Agent`: reusable specialist definition (general profile shape above)
- `Connection`: link to a concrete runtime/backend and its operational details

## Boundary Table

| Field / Concept | Phase 2 General Profile Data | Deferred Connection Or Backend Data |
| --- | --- | --- |
| Identifier | `name` | Remote runtime IDs |
| Presentation | `displayName` | Backend-specific labels/aliases |
| Behavior intent | `role`, `instructions` | ACP/OpenCode launch flags |
| Tool selection | `toolRefs` | Slash-command inventories |
| Runtime defaults | `executionSettings.defaultModel` | Backend transport/options |
| Working directory | No | `cwd` |
| MCP injection | No | `mcpServers` |
| Capability semantics | No | ACP capability flags |
| Session linkage | No | Remote session identifiers |

## Phase 3 Integration Note

Phase 3 can add OpenCode bindings by attaching `Connection`-side data to runs or
runtime sessions while reusing this stable Phase 2 `Agent` profile schema.
