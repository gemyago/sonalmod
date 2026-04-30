---
phase: 03-opencode-coding-lane
plan: "02"
subsystem: api
tags: [opencode, acp, codinglane, launcher, json-rpc]
requires:
  - phase: 03-01
    provides: persisted OpenCode bindings linked to agent profiles
provides:
  - Validated-subset ACP stdio launch transport for OpenCode
  - Launcher service resolving profile + binding into launch requests
  - Typed validation/not-found/launch-failed error mapping for CODE-03
affects: [03-03, runtime/internal/agentapi, apps/sonalmod runtime wiring]
tech-stack:
  added: []
  patterns:
    - JSON-RPC stdio client constrained to initialize/session-new/session-prompt
    - Service-layer launch orchestration with typed domain errors
key-files:
  created:
    - runtime/internal/codinglane/opencode_acp_client.go
    - runtime/internal/codinglane/opencode_launcher.go
    - runtime/internal/codinglane/launch_request_mapper.go
  modified:
    - runtime/internal/codinglane/opencode_acp_client_test.go
    - runtime/internal/codinglane/opencode_launcher_test.go
    - runtime/internal/codinglane/launch_request_mapper_test.go
key-decisions:
  - "Launch path is strictly limited to initialize, session/new, session/prompt, and session/update handling."
  - "Launcher resolves saved profile and binding state first, then maps to ACP request with explicit error-kind classification."
patterns-established:
  - "OpenCodeACPError/OpenCodeLaunchError kinds provide stable API-layer mapping surface."
  - "Mapper composes profile instructions/tool refs/default model into final prompt text while keeping connection fields in bindings."
requirements-completed: [CODE-03]
duration: 12 min
completed: 2026-04-22
---

# Phase 3 Plan 02: OpenCode Launch Service Summary

**Validated OpenCode ACP launch client and profile+binding launcher orchestration with typed failure mapping for CODE-03**

## Performance

- **Duration:** 12 min
- **Started:** 2026-04-22T20:03:40Z
- **Completed:** 2026-04-22T20:15:41Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Implemented runtime ACP transport that performs `initialize -> session/new -> session/prompt` and captures `session/update` notifications.
- Added launcher service resolving saved profile and binding configuration, mapping into ACP launch requests deterministically.
- Added typed error kinds for validation/not-found/protocol/subprocess/launch-failed conditions with deterministic tests.

## Task Commits

1. **Task 1: Build validated-subset ACP transport for OpenCode launches** - `3905256` (test), `e8a369b` (feat)
2. **Task 2: Implement launcher service that resolves saved configuration and reports failures clearly** - `01fc8d3` (test), `f16ad4b` (feat)
3. **Task 2 verification follow-up:** `c677d56` (fix)

## Files Created/Modified
- `runtime/internal/codinglane/opencode_acp_client.go` - ACP subprocess and JSON-RPC launch transport with typed protocol/subprocess/validation errors.
- `runtime/internal/codinglane/opencode_launcher.go` - profile+binding resolution orchestrator and launch error mapping.
- `runtime/internal/codinglane/launch_request_mapper.go` - merge layer from profile and binding defaults into ACP request payloads.
- `runtime/internal/codinglane/opencode_acp_client_test.go` - deterministic helper-process and protocol branch coverage tests.
- `runtime/internal/codinglane/opencode_launcher_test.go` - launcher behavior/error mapping coverage for success, validation, not-found, and launch-failed branches.
- `runtime/internal/codinglane/launch_request_mapper_test.go` - mapper merge and validation tests.

## Decisions Made
- Enforced validated ACP subset by implementation (no runtime usage of cancel/close/load/list methods).
- Kept profile/binding boundary strict: profile contributes behavioral context; binding contributes command/cwd transport defaults.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Runtime lint and file-coverage gates failed after initial implementation**
- **Found during:** Plan verification (`make affected-lint-test`)
- **Issue:** New launcher/client files failed runtime lint rules and per-file coverage threshold.
- **Fix:** Refactored ACP client into smaller helpers, added security/lint-compliant annotations/structure, and expanded tests to cover protocol/error branches until coverage passed.
- **Files modified:** `runtime/internal/codinglane/opencode_acp_client.go`, `runtime/internal/codinglane/opencode_acp_client_test.go`, `runtime/internal/codinglane/opencode_launcher_test.go`, `runtime/internal/codinglane/launch_request_mapper_test.go`
- **Verification:** `npx nx run runtime:lint`, `npx nx run runtime:test`, and `make affected-lint-test`
- **Committed in:** `c677d56`

---

**Total deviations:** 1 auto-fixed (Rule 3 blocking)
**Impact on plan:** Required for task completion protocol compliance; no scope expansion beyond CODE-03 implementation quality gates.

## Issues Encountered
None

## Known Stubs
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Runtime coding-lane launch core is ready for Phase 3 Plan 03 API exposure and app wiring.
- No blockers; 03-03 can consume launcher interfaces directly.

## Self-Check: PASSED
- Verified required files exist on disk.
- Verified all task commit hashes are present in git history.
