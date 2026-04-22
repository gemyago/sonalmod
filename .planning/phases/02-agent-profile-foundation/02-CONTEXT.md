# Phase 2: Agent Profile Foundation - Context

**Gathered:** 2026-04-22
**Status:** Ready for planning
**Source:** Planning synthesis from roadmap, Phase 1 findings, and current runtime architecture

<domain>
## Phase Boundary

This phase exists to define agent profiles as durable first-class data without baking ACP or OpenCode details into the general model too early.

The deliverable is a general profile schema, persistence support, and a control-plane surface to create, edit, and load saved profiles. It is not yet the backend-specific OpenCode execution lane.

</domain>

<decisions>
## Implementation Decisions

### Profile boundary
- Agent profiles must stay general and backend-agnostic in this phase.
- Store role, instructions, selected tool references, and Sonalmod-owned execution settings only.
- Do not store ACP or OpenCode connection details in the general profile shape.

### Phase 1 carry-forward
- Build Phase 2 around the validated ACP subset only: `initialize`, `session/new`, `session/prompt`, and `session/update`.
- Treat cancellation, session resume, session listing, session close, MCP injection behavior, and slash-command guarantees as deferred backend-specific concerns.

### Architecture shape
- Keep durable agent-profile data in `runtime/internal`, parallel to provider config and session storage patterns.
- Reuse the existing file-vs-database storage selector under `agentRuntime` instead of introducing a separate persistence switch.
- Keep `runtime/httpapi` thin and extend the spec-first `runtime/internal/agentapi` layer if profile CRUD is exposed over HTTP.
- Keep `apps/sonalmod` responsible for choosing implementations, wiring services, and mounting the runtime API, not for owning profile persistence logic.

### The agent's Discretion
- Exact profile field names and whether the primary identifier is immutable `name` or another stable key.
- Exact validation depth for tool references, as long as the first slice does not rely on an unimplemented tool-catalog API.
- Exact doc artifact name for the phase's durable schema contract.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project direction
- `./AGENTS.md` - repo-level workflow and completion rules
- `./docs/golang-coding-guide.md` - required Go planning and coding guidance
- `.planning/PROJECT.md` - project framing and current constraints
- `.planning/ROADMAP.md` - phase goal and downstream sequencing
- `.planning/REQUIREMENTS.md` - v1 requirement context
- `.planning/STATE.md` - current focus and project memory

### Phase 1 findings
- `.planning/phases/01-acp-discovery-and-capability-map/01-CONTEXT.md` - Phase 1 planning boundary
- `.planning/phases/01-acp-discovery-and-capability-map/01-RESEARCH.md` - ACP/OpenCode planning research
- `.planning/phases/01-acp-discovery-and-capability-map/01-01-SUMMARY.md` - ACP probe harness implementation outcome
- `.planning/phases/01-acp-discovery-and-capability-map/01-02-SUMMARY.md` - validated OpenCode ACP subset outcome
- `docs/implementation/opencode-acp-capability-map.md` - validated vs deferred ACP behavior

### Existing runtime and app patterns
- `runtime/AGENTS.md` - runtime public-contract and persistence boundaries
- `apps/sonalmod/AGENTS.md` - app wiring and runtime persistence config guidance
- `tests/AGENTS.md` - high-level integration expectations
- `runtime/agent/providers_config.go` - public alias/factory pattern for durable config services
- `runtime/internal/llmproviders/providers_config.go` - provider config domain/service shape
- `runtime/internal/sessions/factory.go` - storage backend selection pattern
- `runtime/httpapi/handler.go` - thin public HTTP wrapper pattern
- `runtime/internal/agentapi/provider_handlers.go` - runtime CRUD handler pattern
- `apps/sonalmod/internal/runtime.go` - service construction and runtime wiring pattern

### Domain vocabulary
- `docs/domain-terminology.md` - canonical distinction between Agent and Connection

</canonical_refs>

<specifics>
## Specific Ideas

- Prefer a new `runtime/internal/agentprofiles` package mirroring the existing provider-config architecture.
- Reuse file and database implementations so saved profiles survive restart in the same storage mode already selected for runtime state.
- Keep tool references lightweight for the first slice, because the runtime does not yet expose a stable tool-catalog API.
- Capture the Phase 2 schema boundary in a durable implementation doc so Phase 3 can add ACP/OpenCode bindings without redefining the general profile contract.

</specifics>

<deferred>
## Deferred Ideas

- OpenCode- or ACP-specific connection details inside the core profile
- Storage of remote session identifiers, capability flags, `cwd`, or `mcpServers`
- User-facing cancellation, session resume, or session listing guarantees
- The actual ACP-backed coding-agent launch path targeted by Phase 3

</deferred>

---

*Phase: 02-agent-profile-foundation*
*Context gathered: 2026-04-22 via planning synthesis*
