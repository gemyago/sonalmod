# Phase 3: OpenCode Coding Lane - Research

**Created:** 2026-04-22  
**Scope:** Repo-local architecture + Phase 1 ACP findings + Phase 2 profile foundations

## Research Question

How should Sonalmod add an OpenCode-backed coding lane that can be selected and launched from saved profiles without violating the validated ACP subset or the Agent-vs-Connection boundary?

## Primary Sources

- `.planning/REQUIREMENTS.md` (`CODE-02`, `CODE-03`, `CODE-04`)
- `.planning/ROADMAP.md` (Phase 3 goal and success criteria)
- `docs/implementation/opencode-acp-capability-map.md`
- `docs/implementation/agent-profile-schema-boundary.md`
- `runtime/AGENTS.md`
- `apps/sonalmod/AGENTS.md`
- `.planning/phases/01-acp-discovery-and-capability-map/01-02-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-01-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-02-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-03-SUMMARY.md`

## Confirmed Facts

### ACP scope constraints

- Validated OpenCode ACP subset is: `initialize`, `session/new`, `session/prompt`, `session/update`.
- `session/cancel` is not advertised in observed OpenCode runs.
- `session/load` and `session/list` are advertised but untested; must remain optional and non-required for Phase 3.

### Existing architecture constraints

- General profile schema already exists and is durable (`runtime/internal/agentprofiles` + API + app wiring).
- Phase 2 explicitly kept backend connection data out of the general profile contract.
- Runtime API expansion pattern is spec-first in `runtime/internal/agentapi/openapi.yaml` with generated code and thin `runtime/httpapi` wrapper.
- App wiring and migration control live in `apps/sonalmod/internal/runtime.go`.

## Planning Implications

1. Phase 3 needs a backend-specific binding layer (Connection-side data) linked to general profiles, not a schema rewrite of general profiles.
2. Launch flow must use only validated ACP calls and explicitly avoid dependence on cancellation/resume/listing.
3. API/UI entry points for "choose saved coding profile and run" should resolve profile + OpenCode binding from persistence so users do not re-enter full config each run.
4. Failure surfacing can be minimal but must produce actionable error paths in API responses/logs.

## Risks And Unknowns

- Real OpenCode environment/auth state is external and may fail at runtime; launch path must return clear operational errors.
- `session/update` event variance can cause brittle parsing if handlers assume fixed payload details.
- If model/tool defaults are split across general profile and OpenCode binding, merge precedence must be explicit and tested.

## Recommended Planning Direction

1. Add OpenCode binding domain + persistence (file/DB) linked to existing profile names (D-01, D-03, D-04).
2. Add ACP launch service that enforces validated subset only and maps failures clearly (D-02).
3. Expose coding-lane config + launch endpoints through runtime API and wire in app startup (D-05).

---

*Research synthesized from local project artifacts on 2026-04-22.*
