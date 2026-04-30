---
phase: 03-opencode-coding-lane
plan: "03"
subsystem: api
tags: [opencode, acp, runtime-api, openapi, go]
requires:
  - phase: 03-01
    provides: OpenCode binding persistence services (file + database)
  - phase: 03-02
    provides: OpenCode launcher and ACP launch execution path
provides:
  - Runtime OpenAPI and handlers for OpenCode binding CRUD
  - Runtime launch endpoint using saved profile+binding selectors
  - App runtime wiring for OpenCode binding and launcher services
  - Integration contract documenting validated and deferred ACP behavior
affects: [runtime/httpapi, apps/sonalmod runtime startup, phase-04 planning]
tech-stack:
  added: []
  patterns:
    - Thin API handlers that map domain services to problem-details responses
    - App runtime dependency wiring by storage backend selector
key-files:
  created:
    - runtime/internal/agentapi/opencode_binding_mapper.go
    - runtime/internal/agentapi/opencode_binding_handlers.go
    - runtime/internal/agentapi/opencode_launch_handlers.go
    - runtime/internal/agentapi/opencode_binding_mapper_test.go
    - runtime/agent/opencode_launcher.go
    - docs/implementation/opencode-coding-lane-contract.md
    - .planning/phases/03-opencode-coding-lane/03-USER-SETUP.md
  modified:
    - runtime/internal/agentapi/openapi.yaml
    - runtime/internal/agentapi/api.gen.go
    - runtime/internal/agentapi/server.go
    - runtime/httpapi/handler.go
    - runtime/httpapi/handler_test.go
    - apps/sonalmod/internal/runtime.go
    - apps/sonalmod/internal/runtime_test.go
key-decisions:
  - "Expose OpenCode binding CRUD and launch on runtime API with camelCase request/response schema."
  - "Require OpenCode services in httpapi handler args to prevent partially wired runtime server startup."
  - "Keep launch endpoint selector-driven (profileName/bindingName/prompt), avoiding full config payload re-entry."
patterns-established:
  - "OpenCode launch handler maps typed launcher error kinds to deterministic HTTP statuses."
  - "Database startup auto-migrates runtime runner, profile service, and OpenCode binding tables together."
requirements-completed: [CODE-02, CODE-03, CODE-04]
duration: 18 min
completed: 2026-04-22
---

# Phase 3 Plan 03: OpenCode Runtime API Summary

**Runtime now supports OpenCode binding CRUD plus selector-based OpenCode launches, wired end-to-end through app startup and documented ACP contract boundaries.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-04-22T20:21:44Z
- **Completed:** 2026-04-22T20:40:02Z
- **Tasks:** 2
- **Files modified:** 16

## Accomplishments
- Added OpenCode binding CRUD and launch endpoints to runtime OpenAPI and generated API surface.
- Implemented runtime handlers/tests for binding lifecycle and launch status/problem-details mapping.
- Wired OpenCode binding service and launcher into `apps/sonalmod` runtime for file/db modes, including DB auto-migrate.
- Published OpenCode coding-lane contract doc covering endpoint behavior, profile/binding persistence split, validated ACP subset, and deferred ACP features.
- Added phase user setup artifact for OpenCode CLI prerequisite.

## Task Commits

1. **Task 1 (TDD RED): OpenCode binding/launch API failing tests** - `1f57aed` (`test`)
2. **Task 1 (TDD GREEN): OpenAPI + handlers + HTTP wiring** - `4630cc4` (`feat`)
3. **Task 2: App runtime wiring + contract documentation** - `3db28ad` (`feat`)
4. **Post-task quality gate fixes:** lint/coverage/refactors required by repository gates - `014b49c` (`fix`)

## Files Created/Modified
- `runtime/internal/agentapi/openapi.yaml` - Added OpenCode binding CRUD and launch endpoint contracts.
- `runtime/internal/agentapi/api.gen.go` - Regenerated API models/routes/server interface.
- `runtime/internal/agentapi/opencode_binding_mapper.go` - Domain-to-API mapping for OpenCode bindings.
- `runtime/internal/agentapi/opencode_binding_handlers.go` - Binding CRUD handlers with deterministic error mapping.
- `runtime/internal/agentapi/opencode_launch_handlers.go` - Launch handler with auth check, selector validation, and typed error mapping.
- `runtime/httpapi/handler.go` - Added required OpenCode service dependencies for handler construction.
- `apps/sonalmod/internal/runtime.go` - Wired OpenCode services for file/db startup and auto-migrate.
- `docs/implementation/opencode-coding-lane-contract.md` - Durable integration contract for OpenCode coding lane.
- `.planning/phases/03-opencode-coding-lane/03-USER-SETUP.md` - Manual setup notes for live OpenCode CLI verification.

## Decisions Made

- Added a public `runtime/agent` launcher alias (`OpenCodeLauncher`) to avoid illegal cross-module imports from `apps/sonalmod` into `runtime/internal/codinglane`.
- Kept handler behavior thin: request decode/validation → domain service invocation → problem-details mapping.
- Mapped launcher error kinds as:
  - `validation` → `400`
  - `not-found` → `404`
  - `launch-failed`/fallback → `500`

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added public launcher alias surface in `runtime/agent`**
- **Found during:** Task 1 implementation
- **Issue:** `apps/sonalmod` cannot import `runtime/internal/codinglane` due Go `internal/` visibility rules; app wiring needed a public launcher type/constructor.
- **Fix:** Added `runtime/agent/opencode_launcher.go` with exported launcher aliases and constructor.
- **Files modified:** `runtime/agent/opencode_launcher.go`, `runtime/agent/opencode_launcher_test.go`
- **Verification:** `go test ./runtime/agent` and full `make affected-lint-test` pass.
- **Committed in:** `4630cc4`

**2. [Rule 3 - Blocking] Expanded tests/refactors to satisfy strict lint and file-coverage gates**
- **Found during:** Final verification (`make affected-lint-test`)
- **Issue:** New runtime files failed lint rules (`golines`, complexity, ireturn) and runtime file coverage thresholds.
- **Fix:** Refactored launch/runtime helper flow, formatted long literals, and added targeted mapper/handler branch tests.
- **Files modified:** `runtime/internal/agentapi/opencode_*`, `runtime/httpapi/handler_test.go`, `apps/sonalmod/internal/runtime.go`
- **Verification:** `make affected-lint-test` passes cleanly.
- **Committed in:** `014b49c`

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Deviations were required to complete app wiring and meet mandatory repository quality gates; no scope creep beyond plan objective.

## Issues Encountered

- OpenAPI path edit initially misplaced `agent-profiles` operations under `/opencode-launches`; corrected and regenerated.
- Repo quality gates enforced strict per-file coverage and formatting; resolved with focused branch coverage and formatting fixes.

## User Setup Required

External runtime verification requires manual setup. See [03-USER-SETUP.md](./03-USER-SETUP.md).

## Known Stubs

None.

## Next Phase Readiness

- Runtime API and app wiring for OpenCode coding lane are complete and verified.
- Ready for downstream functional/UAT verification and broader integration phases.

## Self-Check: PASSED
