# Roadmap: Sonalmod

**Based on:** `.planning/PROJECT.md` and `.planning/REQUIREMENTS.md`
**Primary constraint:** solo-first, OpenCode first, ACP discovery before abstraction freeze, terminology still provisional.

## Phase 1: ACP Discovery And Capability Map

**Goal:** Experiment with ACP through OpenCode and capture the validated ACP subset Sonalmod should design around.

**Why this phase exists:** the current plan knows the first backend and protocol, but not yet enough about their real constraints to lock in the higher-level model safely.

**Requirements covered:**
- CODE-01

**Success criteria:**
- Sonalmod has a written capability map for the subset of ACP exercised through OpenCode. ✓
- The project has at least one experiment that proves the basic lifecycle needed for the first integration. ✓
- Unsupported or unclear ACP behaviors are documented as explicit non-goals for the first slice. ✓
- The planning docs are updated to reflect what the experiment changed about the intended abstraction. ✓

## Phase 2: Agent Profile Foundation

**Goal:** Define and persist general agent profiles as first-class project data, informed by the ACP discovery results.

**Why this phase exists:** once the first protocol boundary is better understood, the harness needs a stable configuration model that does not overfit to guesswork.

**Requirements covered:**
- AGNT-01
- AGNT-02
- AGNT-03
- PERS-01
- PERS-02

**Success criteria:**
- A configurable agent profile can be created with role, instructions, tools, and execution settings.
- Saved profiles survive restart and can be loaded back without loss.
- General agent configuration is stored separately from ACP-specific connection details.
- The project has a clear working schema for agent definitions that reflects Phase 1 findings.

## Phase 3: OpenCode Coding Lane

**Goal:** Add an ACP-backed coding-agent path that targets OpenCode using the validated subset from Phase 1.

**Why this phase exists:** OpenCode is still the first concrete coding backend, but the integration should now be based on observed protocol behavior instead of assumptions.

**Plans:** 3 plans

Plans:
- [x] 03-01-PLAN.md - Add OpenCode binding contracts and persistence separate from general profiles (completed 2026-04-22)
- [x] 03-02-PLAN.md - Implement validated-subset OpenCode ACP launch service (completed 2026-04-22)
- [x] 03-03-PLAN.md - Expose binding and launch APIs and wire app runtime dependencies (completed 2026-04-22)

**Requirements covered:**
- CODE-02
- CODE-03
- CODE-04

**Success criteria:**
- A coding agent can be configured to use OpenCode through ACP.
- The harness can launch that coding agent without re-entering its full configuration every time.
- The OpenCode path only exposes behavior the project has already validated.
- Failures from the OpenCode path are surfaced clearly enough to debug.

## Phase 4: Run Visibility And Control

**Goal:** Make sub-agent execution observable and interruptible.

**Why this phase exists:** once agents can run, the solo operator needs to see what happened and stop work when necessary.

**Requirements covered:**
- EXEC-01
- EXEC-02
- EXEC-03

**Success criteria:**
- Run status is visible while a sub-agent is executing.
- Output and failure details are accessible after the run finishes.
- A running sub-agent invocation can be cancelled or stopped cleanly.
- The harness leaves a useful trail for follow-up runs.

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CODE-01 | Phase 1 | Completed (2026-04-22) |
| AGNT-01 | Phase 2 | Completed (Phase 2 Plan 02-03 complete 2026-04-22) |
| AGNT-02 | Phase 2 | Completed (Phase 2 Plan 02-02 complete 2026-04-22) |
| AGNT-03 | Phase 2 | Completed (Phase 2 Plan 02-03 complete 2026-04-22) |
| PERS-01 | Phase 2 | Completed (Phase 2 Plan 02-03 complete 2026-04-22) |
| PERS-02 | Phase 2 | Completed (Phase 2 Plan 02-02 complete 2026-04-22) |
| CODE-02 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| CODE-03 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| CODE-04 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| EXEC-01 | Phase 4 | Pending |
| EXEC-02 | Phase 4 | Pending |
| EXEC-03 | Phase 4 | Pending |

**Coverage:** 12/12 v1 requirements mapped.

## Current Focus

Phase 4 is next; Phase 3 is complete with Plans 03-01, 03-02, and 03-03 shipped.
