# Sonalmod

## What This Is

Sonalmod is a solo-first harness for building other agent harnesses. The repo already has an embedded agent runtime, session storage, provider config, skills support, and workspace tools; the next step is to add a higher-level configuration layer for configurable agents and an OpenCode-backed coding-agent lane.

This is for one operator who wants to define agent roles, wire them to execution backends, and run them from a single control plane without hardcoding each special case.

## Core Value

One person can define, launch, and evolve multiple kinds of agents from the same harness, including a coding agent backed by OpenCode, without rewriting the orchestration layer each time or guessing what the first external protocol actually supports.

## Requirements

### Validated

- ✓ Core runtime can start and continue streamed agent runs over HTTP — existing runtime
- ✓ Sessions can be stored, read, and listed — existing runtime
- ✓ Provider configuration and model resolution are already supported — existing runtime
- ✓ Skills and workspace filesystem tools can be registered into the runtime — existing runtime

### Active

- [ ] User can configure the first OpenCode-backed coding agent path around ACP capabilities Sonalmod has actually validated.
- [ ] User can define configurable agent profiles with a role, instructions, tools, and execution settings.
- [ ] User can persist and reuse configurable agent profiles across runs.
- [ ] User can define a coding agent profile that targets OpenCode through ACP.
- [ ] User can launch the OpenCode-backed coding agent from the harness.
- [ ] User can observe run status, output, and failure details for each launched sub-agent.
- [ ] The project keeps general agent configuration separate from ACP-specific connection details where that boundary is supported by integration findings.

### Out of Scope

- OpenCode alternatives beyond the first ACP-backed target — focus stays on OpenCode first.
- Multi-user or shared-workspace collaboration — this is a solo project first.
- Finalized domain naming for agent categories — terminology will be refined later.
- A full marketplace or plugin registry for third-party agent packs — too much surface area for v1.

## Context

- Existing codebase already contains a Go agent runtime, HTTP session API, storage, model/provider config, skills discovery, and workspace filesystem tools.
- The current product goal is not to replace that runtime, but to add a higher-level harness that can configure and launch different kinds of agents on top of it.
- The first external coding backend to support is OpenCode, using ACP as the integration boundary.
- ACP behavior and constraints are not yet understood well enough to freeze the final abstraction; the project needs an experiment-first step.
- Terminology is intentionally provisional; "classic sub-agent" and "coding sub-agent" are still working labels, not settled product language.
- Phase 1 probes produced a validated ACP subset in `docs/implementation/opencode-acp-capability-map.md` based on repeatable local ACP runs.

## Constraints

- **Solo-first**: one user is the initial audience, so keep workflows lightweight and direct.
- **Existing platform**: build on the current Go runtime and preserve its minimal public contract, avoiding unnecessary API expansion.
- **Backend focus**: OpenCode is the first coding target, so defer other backends until that path is stable.
- **Experiment first**: validate ACP and OpenCode behavior before locking in profile shape or agent taxonomy.
- **Terminology TBD**: domain language may change later, so avoid baking in confusing names too early.
- **Local-first**: the harness should work cleanly inside this repo and its existing tooling, without assuming shared cloud infra.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Agent harness is the product direction | The user wants a harness of harnesses, not just a single agent runner | — Pending |
| Start with an ACP exploration step | The first backend integration should inform the abstraction instead of being forced into a guessed model | — Pending |
| Treat "general agent" and "coding agent" as a working split, not a final taxonomy | The distinction seems useful, but it should survive contact with ACP before it becomes a hard rule | — Pending |
| Use OpenCode as the first coding backend | Gives a concrete ACP-backed integration target | — Pending |
| Keep terminology provisional for now | Domain vocabulary will be refined after the first slice lands | — Pending |
| Build first OpenCode integration around the validated ACP subset | Probe transcripts showed required request shapes and optional capabilities that remain unvalidated | 2026-04-22 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `$gsd-transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `$gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check - still the right priority?
3. Audit Out of Scope - reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-22 after planning review*
