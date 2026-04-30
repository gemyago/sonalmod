---
phase: 2
slug: agent-profile-foundation
status: draft
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-22
---

# Phase 2 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | `runtime/.testcoverage.yaml`, `apps/sonalmod/.testcoverage.yaml` |
| **Quick run command** | `use the active task's module-scoped <verify> command` |
| **Full suite command** | `make affected-lint-test` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run the active task's module-scoped `<verify>` command
- **After every plan wave:** Run `make affected-lint-test`
- **Before `$gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 2-01-01 | 01 | 1 | AGNT-01, AGNT-03 | T-2-01-01 / T-2-01-02 | General profile schema rejects ACP-specific fields and preserves immutable identifiers | unit | `cd runtime && go test ./agent ./internal/agentprofiles` | ✅ | pending |
| 2-01-02 | 01 | 1 | PERS-01, PERS-02 | T-2-01-03 | File and DB services reload the same saved profile after restart-shaped reconstruction | unit | `cd runtime && go test ./internal/agentprofiles ./agent` | ✅ | pending |
| 2-02-01 | 02 | 2 | AGNT-01 | T-2-02-02 | OpenAPI contract exposes only general profile fields in camelCase and keeps generated artifacts in sync with the spec | docs-check | `cd runtime && go generate ./internal/agentapi && rg -n "/agent-profiles|profileName|toolRefs|executionSettings" internal/agentapi/openapi.yaml internal/agentapi/api.gen.go` | ✅ | pending |
| 2-02-02 | 02 | 2 | AGNT-02, PERS-02 | T-2-02-01 / T-2-02-03 | CRUD handlers reject malformed or conflicting profile writes and preserve runtime auth and wrapper boundaries | integration | `cd runtime && go generate ./internal/agentapi && go test ./internal/agentapi ./httpapi` | ✅ | pending |
| 2-03-01 | 03 | 3 | PERS-01, PERS-02 | T-2-03-01 / T-2-03-02 | App startup wires profile persistence with the existing storage selector and migrates DB state safely | integration | `cd apps/sonalmod && go test ./internal -run TestNewRuntime` | ✅ | pending |
| 2-03-02 | 03 | 3 | AGNT-03 | T-2-03-03 | Durable schema doc preserves the general profile vs deferred connection boundary | docs-check | `test -f docs/implementation/agent-profile-schema-boundary.md && rg -n "^## General Profile Data$|^## Deferred Connection Or Backend Data$|toolRefs|executionSettings|cwd|mcpServers|OpenCode|ACP" docs/implementation/agent-profile-schema-boundary.md` | ✅ | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all missing references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
