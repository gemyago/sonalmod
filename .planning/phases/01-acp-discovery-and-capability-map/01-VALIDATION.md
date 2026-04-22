---
phase: 1
slug: acp-discovery-and-capability-map
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-22
---

# Phase 1 - Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | `tests/agent/integration-cli/go.mod` |
| **Quick run command** | `cd tests/agent/integration-cli && go test ./...` |
| **Full suite command** | `make affected-lint-test` |
| **Estimated runtime** | ~180 seconds |

---

## Sampling Rate

- **After every task commit:** Run `cd tests/agent/integration-cli && go test ./...`
- **After every plan wave:** Run `make affected-lint-test`
- **Before `$gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 180 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 1-01-01 | 01 | 1 | CODE-01 | - | ACP mode in integration-cli negotiates capabilities without writing invalid JSON-RPC to downstream stdio | unit | `cd tests/agent/integration-cli && go test ./...` | ❌ W0 | pending |
| 1-01-02 | 01 | 1 | CODE-01 | - | Transcript writer records request and response envelopes deterministically | unit | `cd tests/agent/integration-cli && go test ./...` | ❌ W0 | pending |
| 1-02-01 | 02 | 2 | CODE-01 | - | OpenCode capability map is derived from captured transcripts, not memory | integration | `cd tests/agent/integration-cli && go test ./...` | ❌ W0 | pending |
| 1-02-02 | 02 | 2 | CODE-01 | - | Planning docs reflect the validated ACP subset and deferred features | docs-check | `rg -n "validated|deferred|unsupported" docs/implementation .planning/phases/01-acp-discovery-and-capability-map` | ❌ W0 | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

- [ ] `tests/agent/integration-cli/acp_client_test.go` - ACP lifecycle and capability parsing coverage
- [ ] `tests/agent/integration-cli/acp_transcript_test.go` - transcript capture coverage
- [ ] `tests/agent/integration-cli/main.go` - ACP subcommand wiring

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| OpenCode starts under ACP with real local credentials and environment | CODE-01 | Requires local `opencode` installation and authenticated setup outside repo automation | Run the probe against `opencode acp`, capture transcript, confirm initialize and session lifecycle complete without protocol framing errors |
| Capability map matches observed OpenCode behavior | CODE-01 | Requires judgment over captured transcript content | Review the generated capability map against stored transcript files and confirm every supported or unsupported claim cites an observed exchange |

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all missing references
- [ ] No watch-mode flags
- [ ] Feedback latency < 180s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
