---
phase: 01-acp-discovery-and-capability-map
plan: "02"
subsystem: opencode-acp-discovery
tags: [acp, opencode, capability-map, planning]
requires: ["01-01"]
provides:
  - OpenCode ACP experiment procedure for initialize/prompt and cancel attempts
  - Capability map classifying validated, advertised but untested, not advertised, and deferred ACP features
  - Planning updates aligned to the validated ACP subset
affects: [opencode-acp-experiments, planning]
key-files:
  created:
    - docs/implementation/opencode-acp-experiments.md
    - docs/implementation/opencode-acp-capability-map.md
  modified:
    - tests/agent/integration-cli/acp_client.go
    - tests/agent/integration-cli/acp_client_test.go
    - .planning/PROJECT.md
    - .planning/ROADMAP.md
    - .planning/STATE.md
requirements-completed: [CODE-01]
completed: 2026-04-22
---

# Phase 01 Plan 02 Summary

## Outcome

OpenCode ACP probing is complete and Sonalmod now has a validated ACP subset grounded in reproducible local runs.

## What Was Delivered

- Ran ACP probes against `opencode acp` and documented reproducible commands for re-running on demand.
- Published reproducible run notes in:
  - `docs/implementation/opencode-acp-experiments.md`
- Published capability classification map in:
  - `docs/implementation/opencode-acp-capability-map.md`
- Updated `.planning/PROJECT.md`, `.planning/ROADMAP.md`, and `.planning/STATE.md` so future phases explicitly build from the validated ACP subset.

## Key Findings

- Validated: `initialize`, `session/new`, `session/prompt`, `session/update` streaming.
- Advertised but untested: `session/load`, `session/list`, `session resume/fork`.
- Not advertised/available in this run: `session/cancel` (method not found), `session/close`.
- Deferred: MCP server injection behavior with non-empty `mcpServers`, slash-command support boundaries.

## Deviations

The probe client from Plan `01-01` required compatibility fixes discovered during real OpenCode runs:

- `initialize` now sends `protocolVersion`.
- `session/new` now sends required `cwd` and `mcpServers`.
- `session/prompt` now sends prompt content as an array of text blocks.

These fixes were implemented in `tests/agent/integration-cli/acp_client.go` with test coverage updates in `tests/agent/integration-cli/acp_client_test.go`.
