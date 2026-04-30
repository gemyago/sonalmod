---
phase: 3
slug: opencode-coding-lane
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-22
---

# Phase 3 - Validation Strategy

## Test Infrastructure

| Property | Value |
|----------|-------|
| Framework | go test |
| Quick run command | use the active task's module-scoped `<verify>` command |
| Full suite command | `make affected-lint-test` |
| Estimated runtime | ~180 seconds |

## Sampling Rate

- After every task commit: run that task's `<automated>` verify command.
- After every wave: run `make affected-lint-test`.
- Before phase verification: all plan-level verify commands + full suite must pass.

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Automated Command |
|---------|------|------|-------------|-------------------|
| 3-01-01 | 03-01 | 1 | CODE-02 | `cd runtime && go test ./agent ./internal/codinglane` |
| 3-01-02 | 03-01 | 1 | CODE-04 | `cd runtime && go test ./internal/codinglane ./internal/agentapi` |
| 3-02-01 | 03-02 | 2 | CODE-03 | `cd runtime && go test ./internal/codinglane -run TestOpenCodeACPLauncher` |
| 3-02-02 | 03-02 | 2 | CODE-03 | `cd runtime && go test ./internal/codinglane ./internal/agentapi` |
| 3-03-01 | 03-03 | 3 | CODE-02, CODE-04 | `cd runtime && go generate ./internal/agentapi && go test ./internal/agentapi ./httpapi` |
| 3-03-02 | 03-03 | 3 | CODE-03 | `cd apps/sonalmod && go test ./internal -run TestNewRuntime` |

## Source Coverage Audit

| Source Type | Item | Covered By |
|-------------|------|------------|
| GOAL | Phase 3 OpenCode ACP coding lane | 03-01, 03-02, 03-03 |
| REQ | CODE-02 | 03-01, 03-03 |
| REQ | CODE-03 | 03-02, 03-03 |
| REQ | CODE-04 | 03-01, 03-03 |
| RESEARCH | Validated ACP subset only | 03-02 task actions (D-02) |
| RESEARCH | Agent-vs-Connection split | 03-01 task actions (D-03) |
| CONTEXT | D-01 scoped to CODE-02/03/04 | all plans |
| CONTEXT | D-02 validated ACP methods only | 03-02, 03-03 |
| CONTEXT | D-03 preserve profile boundary | 03-01, 03-03 |
| CONTEXT | D-04 architecture pattern reuse | all plans |
| CONTEXT | D-05 execute-ready plan structure | this plan set |

## Validation Sign-Off

- [ ] All task verify commands pass
- [ ] Wave checks pass (`make affected-lint-test` per wave)
- [ ] No task relies on deferred ACP features
- [ ] API and runtime wiring preserve Phase 2 profile boundary

