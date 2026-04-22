# Phase 3: OpenCode Coding Lane - Context

**Gathered:** 2026-04-22  
**Status:** Ready for planning and execution

<domain>
## Phase Boundary

Phase 3 adds the first production coding lane that uses OpenCode through ACP, built only on the subset validated in Phase 1 and reusing the profile foundation from Phase 2.

This phase delivers backend binding, launch flow, and profile selection for OpenCode coding runs. It does not expand to unvalidated ACP features or additional coding backends.

</domain>

<decisions>
## Decisions (Locked)

- **D-01:** Scope is limited to `CODE-02`, `CODE-03`, and `CODE-04` for this phase.
- **D-02:** OpenCode ACP integration must rely only on validated ACP behavior: `initialize`, `session/new`, `session/prompt`, and `session/update`.
- **D-03:** Keep Phase 2 boundary intact: general agent profile data stays separate from backend-specific connection/binding data.
- **D-04:** Reuse existing runtime/app architecture patterns (`runtime/internal` domain services, thin `runtime/httpapi`, app wiring in `apps/sonalmod`).
- **D-05:** Produce execute-phase-ready artifacts with wave/dependency frontmatter and automated verification per task.

## Claude's Discretion

- Concrete names for OpenCode binding entities and APIs (must remain consistent with glossary and prior runtime naming patterns).
- Exact package/file layout for coding-lane internals as long as boundaries and requirements stay intact.
- Whether launch endpoints are synchronous or streaming, as long as launch is functional and failures are surfaced clearly.

</decisions>

<deferred>
## Deferred Ideas (Out Of Scope Here)

- Additional coding backends beyond OpenCode.
- ACP features not validated in Phase 1 (`session/cancel`, `session/close`, non-empty `mcpServers` behavior, slash-command guarantees).
- Full run visibility/inspection/cancellation UX beyond what is minimally needed to surface launch failures (reserved for Phase 4 `EXEC-*`).

</deferred>

<canonical_refs>
## Canonical References

- `AGENTS.md`
- `docs/golang-coding-guide.md`
- `.planning/PROJECT.md`
- `.planning/ROADMAP.md`
- `.planning/REQUIREMENTS.md`
- `.planning/STATE.md`
- `.planning/phases/01-acp-discovery-and-capability-map/01-02-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-01-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-02-SUMMARY.md`
- `.planning/phases/02-agent-profile-foundation/02-03-SUMMARY.md`
- `docs/implementation/opencode-acp-capability-map.md`
- `docs/implementation/agent-profile-schema-boundary.md`
- `runtime/AGENTS.md`
- `apps/sonalmod/AGENTS.md`
- `tests/AGENTS.md`

</canonical_refs>
