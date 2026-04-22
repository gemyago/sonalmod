---
phase: 01-acp-discovery-and-capability-map
plan: "01"
subsystem: testing
tags: [acp, integration-cli, json-rpc, go, transcripts]
requires: []
provides:
  - ACP subcommand in integration CLI for stdio JSON-RPC probing
  - Capability-aware ACP session lifecycle (initialize/new/load/prompt/cancel)
  - JSONL transcript recorder for outbound and inbound ACP envelopes
  - Unit test coverage for ACP flow and failure paths
affects: [opencode-acp-experiments, testing]
tech-stack:
  added: []
  patterns: [newline-delimited JSON-RPC over stdio, capability-gated session/load]
key-files:
  created:
    - tests/agent/integration-cli/acp_cmd.go
    - tests/agent/integration-cli/acp_client.go
    - tests/agent/integration-cli/acp_transcript.go
    - tests/agent/integration-cli/acp_cmd_test.go
    - tests/agent/integration-cli/acp_client_test.go
    - tests/agent/integration-cli/acp_transcript_test.go
  modified:
    - tests/agent/integration-cli/main.go
    - tests/agent/integration-cli/main_test.go
    - tests/AGENTS.md
key-decisions:
  - "Reuse tests/agent/integration-cli binary with an acp subcommand instead of introducing a new module."
  - "Treat session/load as capability-gated and fall back to session/new when unsupported."
  - "Record both outbound requests and inbound responses/notifications as JSONL envelopes."
patterns-established:
  - "ACP probes are executed through integration-cli acp with explicit agent command/args."
  - "ACP transport logic is tested with deterministic newline-delimited in-memory scripts."
requirements-completed: [CODE-01]
duration: 1h10m
completed: 2026-04-22
---

# Phase 01 Summary

**Integration CLI now includes a reusable ACP probe mode with capability-aware session lifecycle and transcript capture for OpenCode experiments.**

## Performance

- **Duration:** 1h10m
- **Started:** 2026-04-22T07:35:00Z
- **Completed:** 2026-04-22T08:45:48Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Added `integration-cli acp` command wiring with required ACP probe flags.
- Implemented ACP client lifecycle for `initialize`, `session/new` or `session/load`, `session/prompt`, and optional `session/cancel`.
- Added JSONL transcript recording and extensive tests covering protocol flow, framing, and error handling.
- Updated `tests/AGENTS.md` with manual ACP probe instructions including `opencode acp` prerequisites.

## Task Commits

1. **Task 1: Extend integration-cli with ACP subcommand and transport client** - not committed (workspace changes only)
2. **Task 2: Add ACP transcript capture, tests, and run instructions** - not committed (workspace changes only)

## Files Created/Modified
- `tests/agent/integration-cli/main.go` - Added ACP command and flags to CLI root.
- `tests/agent/integration-cli/acp_cmd.go` - ACP command execution path and output formatting.
- `tests/agent/integration-cli/acp_client.go` - JSON-RPC stdio ACP client and lifecycle logic.
- `tests/agent/integration-cli/acp_transcript.go` - JSONL transcript recorder for ACP envelopes.
- `tests/agent/integration-cli/main_test.go` - ACP command registration coverage.
- `tests/agent/integration-cli/acp_cmd_test.go` - ACP command execution tests via injected dependencies.
- `tests/agent/integration-cli/acp_client_test.go` - ACP client lifecycle/error/transport tests.
- `tests/agent/integration-cli/acp_transcript_test.go` - Transcript serialization and file helper tests.
- `tests/AGENTS.md` - Operator instructions for integration-cli ACP probe mode.

## Decisions Made
- Reused existing integration CLI entrypoint for ACP workflows to keep operations centralized.
- Kept session loading conditional on advertised capabilities to avoid unsupported method calls.
- Captured ACP traffic as structured JSON lines for deterministic replay and capability analysis.

## Deviations from Plan

None - plan executed as specified.

## Issues Encountered
- Initial ACP implementation failed module lint/test gates due strict complexity and per-file coverage thresholds; resolved by refactoring ACP client flow and adding focused tests to satisfy module quality bars.

## User Setup Required

None - no external service configuration required for this code delivery.

## Next Phase Readiness
- Wave 1 deliverable is complete and validated.
- Phase can proceed to Wave 2 (`01-02`) to run real OpenCode ACP probes and publish capability map artifacts.

---
*Phase: 01-acp-discovery-and-capability-map*
*Completed: 2026-04-22*
