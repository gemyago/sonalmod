---
phase: 02-agent-profile-foundation
plan: "03"
subsystem: api
tags: [agent-profiles, runtime, migration, persistence, opencode, acp]
requires:
  - phase: 02-01
    provides: runtime profile persistence services and storage backends
  - phase: 02-02
    provides: runtime profile CRUD HTTP surface
provides:
  - app-layer wiring for profile service in runtime startup
  - database migration orchestration for profile persistence tables
  - durable phase-2 profile schema and boundary contract document
affects: [phase-03-opencode-integration, runtime-http-wiring, profile-persistence]
tech-stack:
  added: []
  patterns:
    - reuse existing agentRuntime storage selector for new runtime persistence services
    - run storage migrations in startup gate before serving runtime API
key-files:
  created:
    - docs/implementation/agent-profile-schema-boundary.md
  modified:
    - apps/sonalmod/internal/runtime.go
    - apps/sonalmod/internal/runtime_test.go
key-decisions:
  - "Profile persistence wiring in apps/sonalmod must use the same agentRuntime storage selector as runner/provider persistence."
  - "Profile DB migration runs in the same startup auto-migrate gate as runner migrations and is skipped when autoMigrate is false."
  - "Phase 2 profile schema explicitly excludes ACP/OpenCode connection details to preserve Agent vs Connection boundary."
patterns-established:
  - "Runtime app wiring pattern: construct service helper + inject into httpapi.NewHandler + migrate in DB startup gate."
  - "Boundary docs pattern: capture shipped schema vs deferred backend fields before integration phases."
requirements-completed: [AGNT-02, AGNT-03, PERS-01, PERS-02]
duration: 18 min
completed: 2026-04-22
---

# Phase 2 Plan 03: Runtime Wiring And Profile Boundary Summary

**Runtime startup now wires profile CRUD through the existing runtime contracts and migrates profile DB tables when auto-migrate is enabled, with a durable schema-vs-connection boundary document for Phase 3.**

## Performance

- **Duration:** 18 min
- **Started:** 2026-04-22T18:54:25Z
- **Completed:** 2026-04-22T19:12:25Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Added profile-service database migration to app runtime startup under the existing `agentRuntime` DB auto-migrate gate.
- Expanded `TestNewRuntime` coverage for file and database startup paths, including migrate-enabled and migrate-disabled behavior for profile tables.
- Authored `docs/implementation/agent-profile-schema-boundary.md` with explicit `Agent` vs `Connection` boundary and deferred ACP/OpenCode fields.

## Task Commits

1. **Task 1: Wire agent profile services into app runtime startup and migration flow** - `9343195` (feat)
2. **Task 2: Publish the durable Phase 2 profile schema and boundary contract** - `5163002` (docs)

## Files Created/Modified
- `apps/sonalmod/internal/runtime.go` - runs `AgentProfilesService.AutoMigrate()` in DB startup auto-migrate flow and keeps profile service wiring on existing runtime storage config.
- `apps/sonalmod/internal/runtime_test.go` - verifies DB startup migrates profile table when enabled and skips migration when disabled.
- `docs/implementation/agent-profile-schema-boundary.md` - durable contract for general profile data vs deferred backend/connection data.

## Decisions Made
- Reused `agentRuntime.storage.type` as the only storage selector for profile persistence wiring (no parallel selector introduced).
- Kept startup migration deterministic by running profile migration in the same DB auto-migrate startup gate before serving requests.
- Declared ACP/OpenCode runtime binding data (`cwd`, `mcpServers`, capability/session specifics) as deferred `Connection` data, not Phase 2 profile schema.

## Verification
- `cd apps/sonalmod && go test ./internal -run TestNewRuntime` -> PASS
- `rg -n "AgentProfilesService|AutoMigrate" apps/sonalmod/internal/runtime.go` -> PASS
- `rg -n "^## General Profile Data$|^## Deferred Connection Or Backend Data$|toolRefs|executionSettings|cwd|mcpServers|OpenCode|ACP" docs/implementation/agent-profile-schema-boundary.md` -> PASS
- `make affected-lint-test` -> PASS
- AGENTS updates -> no changes needed (no command/workflow/architecture guidance changes required)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Phase 2 profile foundation now has app wiring, migration behavior, and schema boundary documentation required for OpenCode binding work in Phase 3.

## Self-Check: PASSED

- FOUND: `.planning/phases/02-agent-profile-foundation/02-03-SUMMARY.md`
- FOUND: `9343195`
- FOUND: `5163002`
