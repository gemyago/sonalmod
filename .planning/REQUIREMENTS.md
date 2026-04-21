# Requirements: Sonalmod

**Defined:** 2026-04-21
**Core Value:** One person can define, launch, and evolve multiple kinds of agents from the same harness, including a coding agent backed by OpenCode, without rewriting the orchestration layer each time.

## v1 Requirements

### Agent Profiles

- [ ] **AGNT-01**: User can define a classic sub-agent profile with a role, instructions, tool set, and execution settings.
- [ ] **AGNT-02**: User can persist and reuse classic sub-agent profiles across runs.
- [ ] **AGNT-03**: User can keep classic sub-agent configuration separate from coding-agent transport details.

### Coding Agents

- [ ] **CODE-01**: User can define a coding sub-agent profile that targets OpenCode through ACP.
- [ ] **CODE-02**: User can launch the OpenCode-backed coding sub-agent from the harness.
- [ ] **CODE-03**: User can choose the OpenCode coding sub-agent for a run without redefining its configuration each time.

### Execution

- [ ] **EXEC-01**: User can see each sub-agent run's status while it executes.
- [ ] **EXEC-02**: User can inspect run output and failure details after execution.
- [ ] **EXEC-03**: User can stop or cancel a running sub-agent invocation.

### Persistence

- [ ] **PERS-01**: Sub-agent definitions persist across restarts.
- [ ] **PERS-02**: Sub-agent configuration can be edited without losing previously saved definitions.

## v2 Requirements

### Expansion

- **BACK-01**: Support additional coding backends beyond OpenCode.
- **TEAM-01**: Support shared or multi-user agent management workflows.
- **MARK-01**: Add a registry or marketplace for reusable agent packs.
- **COMP-01**: Allow more advanced cross-agent composition and delegation patterns.

## Out of Scope

| Feature | Reason |
|---------|--------|
| Additional coding backends beyond OpenCode | Focus the first integration on one ACP path so the abstraction stays honest |
| Multi-user collaboration workflows | Solo-first project; shared ownership adds design overhead too early |
| Public marketplace for agent packs | Discovery and governance are unnecessary before the first agent types work |
| Finalized terminology for agent categories | The current labels are working names only and may change after the first slice |

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

**Coverage:**
- v1 requirements: 11 total
- Mapped to phases: 11
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-21*
*Last updated: 2026-04-21 after initial definition*
