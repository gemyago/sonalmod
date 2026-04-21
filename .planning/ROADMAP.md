# Roadmap: Sonalmod

**Based on:** `.planning/PROJECT.md` and `.planning/REQUIREMENTS.md`
**Primary constraint:** solo-first, OpenCode first, terminology still provisional.

## Phase 1: Agent Profile Foundation

**Goal:** Define and persist classic sub-agent profiles as first-class project data.

**Why this phase exists:** the harness needs a stable way to describe "classic" agents before any coding-backend work can slot into place.

**Requirements covered:**
- AGNT-01
- AGNT-02
- AGNT-03
- PERS-01
- PERS-02

**Success criteria:**
- A classic sub-agent profile can be created with role, instructions, tools, and execution settings.
- Saved profiles survive restart and can be loaded back without loss.
- Classic agent configuration is stored separately from execution backend settings.
- The project has a clear working schema for agent definitions.

## Phase 2: OpenCode Coding Lane

**Goal:** Add an ACP-backed coding sub-agent path that targets OpenCode.

**Why this phase exists:** OpenCode is the first concrete coding backend, so the harness needs one real integration to prove the abstraction.

**Requirements covered:**
- CODE-01
- CODE-02
- CODE-03

**Success criteria:**
- A coding sub-agent can be configured to use OpenCode through ACP.
- The harness can launch that coding sub-agent without re-entering its full configuration every time.
- Failures from the OpenCode path are surfaced clearly enough to debug.
- The coding lane stays distinct from classic sub-agent configuration.

## Phase 3: Run Visibility And Control

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
| AGNT-01 | Phase 1 | Pending |
| AGNT-02 | Phase 1 | Pending |
| AGNT-03 | Phase 1 | Pending |
| PERS-01 | Phase 1 | Pending |
| PERS-02 | Phase 1 | Pending |
| CODE-01 | Phase 2 | Pending |
| CODE-02 | Phase 2 | Pending |
| CODE-03 | Phase 2 | Pending |
| EXEC-01 | Phase 3 | Pending |
| EXEC-02 | Phase 3 | Pending |
| EXEC-03 | Phase 3 | Pending |

**Coverage:** 11/11 v1 requirements mapped.

## Current Focus

Phase 1 is the next planning target.

