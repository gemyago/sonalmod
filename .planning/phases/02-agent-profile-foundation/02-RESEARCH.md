# Phase 2: Agent Profile Foundation - Research

**Created:** 2026-04-22
**Scope:** Current Sonalmod runtime/app architecture and Phase 1 ACP findings relevant to agent profile planning

## Research Question

How should Sonalmod define and persist general agent profiles now that Phase 1 has validated only a narrow ACP subset?

## Primary Sources

- `.planning/ROADMAP.md`
- `.planning/REQUIREMENTS.md`
- `.planning/STATE.md`
- `docs/implementation/opencode-acp-capability-map.md`
- `docs/domain-terminology.md`
- `runtime/AGENTS.md`
- `apps/sonalmod/AGENTS.md`
- `runtime/agent/providers_config.go`
- `runtime/internal/llmproviders/providers_config.go`
- `runtime/internal/sessions/factory.go`
- `runtime/internal/agentapi/provider_handlers.go`
- `runtime/httpapi/handler.go`
- `apps/sonalmod/internal/runtime.go`

## Confirmed Facts

### Product and phase boundary

- Phase 2 must satisfy `AGNT-01`, `AGNT-02`, `AGNT-03`, `PERS-01`, and `PERS-02`.
- The roadmap defines this phase as a foundation for general agent profiles, not as the ACP-backed coding-agent execution path.
- The domain glossary treats `Agent` and `Connection` as separate concepts, which supports keeping general profile data separate from backend-specific connection data.

### Phase 1 implications

- The validated ACP subset is limited to `initialize`, `session/new`, `session/prompt`, and `session/update`.
- `session/cancel` is not advertised in the observed OpenCode runs.
- `session/load` and `session/list` are advertised but were not validated for Sonalmod's first integration slice.
- Phase 2 should not encode assumptions about unsupported or unvalidated ACP behavior into the general profile schema.

### Existing architecture patterns

- Durable agent-adjacent data already lives inside `runtime/internal`, with thin public aliases and constructors exposed from `runtime/agent`.
- `apps/sonalmod` chooses file vs database storage and wires services into the runtime handler, but it does not own persistence implementations.
- Runtime HTTP expansion follows a spec-first pattern through `runtime/internal/agentapi/openapi.yaml`, generated API bindings, hand-written server handlers, and a thin `runtime/httpapi` wrapper.
- The existing provider-config feature already demonstrates the dominant project pattern for durable config entities with file storage, database storage, API exposure, and app wiring.

## Planning Implications

### Recommended architectural home

- Create a new `runtime/internal/agentprofiles` package for the profile domain and storage implementations.
- Add thin public aliases and constructors in `runtime/agent`, matching the provider-config pattern.
- If this phase exposes CRUD over HTTP, extend `runtime/internal/agentapi` and `runtime/httpapi`, then wire the service from `apps/sonalmod/internal/runtime.go`.

### Recommended schema boundary

- Keep `AgentProfile` general-only: role, instructions, selected tool references, and execution settings Sonalmod can interpret without ACP-specific knowledge.
- Do not store `cwd`, `mcpServers`, ACP capability flags, remote session IDs, or OpenCode-specific connection settings in the general profile.
- Keep the profile-vs-connection boundary explicit so Phase 3 can add OpenCode bindings without reshaping the Phase 2 schema.

### Recommended persistence strategy

- Reuse the existing `agentRuntime.storage.type` switch so profiles persist through the same file or database mode already used for runtime state.
- Mirror provider-config service structure: domain types, explicit errors, file implementation, database implementation, tests next to implementation, and public factories.

## Risks And Unknowns

- Database auto-migration currently flows through `runner.AutoMigrate()` for session storage, but provider-config database migration is not centrally orchestrated. Phase 2 needs an explicit strategy for profile-table migration.
- `ToolsRegistry` does not expose a stable public catalog of selectable tool IDs, so strict tool-reference validation may need to stay lightweight in the first slice.
- The repo has no established generic execution-settings contract beyond provider/model resolution and tool registration, so Phase 2 should avoid locking in backend-shaped settings too early.
- The profile identifier shape is still an open decision. Reusing an immutable technical `name` matches provider-config patterns, but this should be treated as an explicit design choice during planning.

## Recommended Planning Direction

1. Build the `AgentProfile` domain and persistence layer in `runtime/internal/agentprofiles`, including file and database implementations plus tests.
2. Expose profile CRUD through the runtime OpenAPI and HTTP stack, following provider-handler conventions and keeping the public HTTP wrapper thin.
3. Wire the profile service in `apps/sonalmod`, add the missing profile-table migration path, and publish a durable schema/boundary doc to anchor Phase 3.

## Validation Architecture

- Automated checks should be split by module boundary:
  - Runtime domain and storage tests in `runtime`
  - Runtime API and HTTP handler tests in `runtime`
  - App wiring and migration orchestration tests in `apps/sonalmod`
- Fast feedback commands should stay module-scoped during execution, with repo-wide gating reserved for plan-wave and completion checks.
- Persistence verification must include restart-shaped coverage: save a profile, construct a fresh service/runtime using the same backing storage, and confirm the profile loads back unchanged.
- Edit flows must verify updates preserve unrelated fields and do not collapse the general-profile vs backend-binding boundary.
- Manual verification should be minimal; the phase should rely mostly on Go tests and generated OpenAPI/runtime handler coverage.

---

*Research based on repo-local architecture and Phase 1 artifacts reviewed on 2026-04-22.*
