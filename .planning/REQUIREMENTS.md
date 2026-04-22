# Requirements: Sonalmod

**Defined:** 2026-04-21
**Core Value:** One person can define, launch, and evolve multiple kinds of agents from the same harness, including a coding agent backed by OpenCode, without rewriting the orchestration layer each time or guessing what the first external protocol actually supports.

## v1 Requirements

### Agent Profiles

- [x] **AGNT-01**: User can define a configurable agent profile with a role, instructions, tool set, and execution settings.
- [x] **AGNT-02**: User can persist and reuse configurable agent profiles across runs.
- [x] **AGNT-03**: User can keep general agent configuration separate from ACP-specific connection details where that boundary is supported by integration findings.

### Coding Agents

- [ ] **CODE-01**: User can configure an OpenCode-backed coding agent using only ACP capabilities Sonalmod has validated through experiment.
- [x] **CODE-02**: User can define a coding agent profile that targets OpenCode through ACP.
- [x] **CODE-03**: User can launch the OpenCode-backed coding agent from the harness.
- [x] **CODE-04**: User can choose the OpenCode coding agent for a run without redefining its configuration each time.

### Execution

- [ ] **EXEC-01**: User can see each sub-agent run's status while it executes.
- [ ] **EXEC-02**: User can inspect run output and failure details after execution.
- [ ] **EXEC-03**: User can stop or cancel a running sub-agent invocation.

### Persistence

- [x] **PERS-01**: Sub-agent definitions persist across restarts.
- [x] **PERS-02**: Sub-agent configuration can be edited without losing previously saved definitions.

## v2 Requirements

### Expansion

- **BACK-01**: Support additional coding backends beyond OpenCode.
- **ACP-01**: Support broader ACP capabilities beyond the first validated subset.
- **TEAM-01**: Support shared or multi-user agent management workflows.
- **MARK-01**: Add a registry or marketplace for reusable agent packs.
- **COMP-01**: Allow more advanced cross-agent composition and delegation patterns.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Additional coding backends beyond OpenCode | Focus the first integration on one ACP path so the abstraction stays honest |
| ACP surface area not validated by experiment | Avoid committing the product model to protocol features we have not yet exercised |
| Multi-user collaboration workflows | Solo-first project; shared ownership adds design overhead too early |
| Public marketplace for agent packs | Discovery and governance are unnecessary before the first agent types work |
| Finalized terminology for agent categories | The current labels are working names only and may change after the first slice |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| CODE-01 | Phase 1 | Pending |
| AGNT-01 | Phase 2 | Completed (2026-04-22) |
| AGNT-02 | Phase 2 | Pending |
| AGNT-03 | Phase 2 | Completed (2026-04-22) |
| PERS-01 | Phase 2 | Completed (2026-04-22) |
| PERS-02 | Phase 2 | Completed (2026-04-22) |
| CODE-02 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| CODE-03 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| CODE-04 | Phase 3 | Completed (Phase 3 Plan 03-03 complete 2026-04-22) |
| EXEC-01 | Phase 4 | Pending |
| EXEC-02 | Phase 4 | Pending |
| EXEC-03 | Phase 4 | Pending |

**Coverage:**
- v1 requirements: 12 total
- Mapped to phases: 12
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-21*
*Last updated: 2026-04-22 after planning review*
