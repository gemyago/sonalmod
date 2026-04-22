# Phase 1: ACP Discovery And Capability Map - Context

**Gathered:** 2026-04-22
**Status:** Ready for planning
**Source:** Planning synthesis from project initialization and follow-up review

<domain>
## Phase Boundary

This phase exists to learn what OpenCode actually exposes through ACP before Sonalmod freezes a higher-level integration model.

The deliverable is not a production-grade multi-backend abstraction. The deliverable is a validated capability map, a reusable probe harness, and an updated project direction based on observed ACP behavior.

</domain>

<decisions>
## Implementation Decisions

### Integration scope
- OpenCode is the only external coding agent backend in scope for this phase.
- ACP is the protocol boundary to investigate.
- The phase should prefer an experiment harness that can be re-run over one-off manual testing.

### Product shaping
- Do not finalize the long-term agent taxonomy in this phase.
- Treat "general agent" and "coding agent" as working categories only.
- Use OpenCode findings to decide which parts of the future configuration model are general and which are ACP-specific.

### Discovery goals
- Validate the ACP lifecycle Sonalmod needs most: initialize, session creation, prompt execution, progress updates, cancellation, and session resume if available.
- Capture actual OpenCode capabilities returned at runtime instead of assuming optional ACP features are supported.
- Record unsupported or unclear features as explicit non-goals for the first implementation slice.

### Implementation shape
- Keep the experiment harness in-repo so future phases can reuse it.
- Prefer Go for the probe tooling to match the existing repo and test patterns.
- Extend `tests/agent/integration-cli` instead of creating a separate ACP test project, so successful experiments become the basis of long-term integration testing.

### The agent's Discretion
- Exact package layout for ACP-specific code inside `tests/agent/integration-cli`.
- Exact file names for experiment transcripts and capability-map docs, as long as they are stable and discoverable.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project direction
- `./AGENTS.md` - repo-level workflow and completion rules
- `./docs/golang-coding-guide.md` - required Go planning and coding guidance
- `.planning/PROJECT.md` - current project framing and constraints
- `.planning/ROADMAP.md` - phase goal and downstream sequencing
- `.planning/REQUIREMENTS.md` - v1 requirement context
- `.planning/STATE.md` - current focus and project memory

### Existing runtime and testing patterns
- `runtime/AGENTS.md` - runtime module boundaries and public contract rules
- `apps/sonalmod/AGENTS.md` - embedded app/runtime wiring guidance
- `tests/AGENTS.md` - integration test expectations and manual setup stance
- `tests/agent/integration-cli/main.go` - existing Go CLI pattern for agent testing
- `tests/agent/integration-cli/cli.go` - existing runner invocation pattern

### Domain vocabulary
- `docs/domain-terminology.md` - canonical product terminology and ACP-related vocabulary

</canonical_refs>

<specifics>
## Specific Ideas

- Probe OpenCode via `opencode acp` over JSON-RPC stdio.
- Capture raw request/response and notification transcripts so Sonalmod can reason about exact protocol behavior later.
- Check capability negotiation before assuming support for `session/load`, `session/list`, or `session/close`.
- Focus on the smallest validated ACP subset Sonalmod needs for its first OpenCode integration.

</specifics>

<deferred>
## Deferred Ideas

- Support for additional coding backends beyond OpenCode
- Final naming for agent categories
- UI or marketplace work for agent configuration
- Broader ACP surface area not exercised by this phase

</deferred>

---

*Phase: 01-acp-discovery-and-capability-map*
*Context gathered: 2026-04-22 via planning synthesis*
