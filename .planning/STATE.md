---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
last_updated: "2026-04-22T20:55:00.000Z"
progress:
  total_phases: 4
  completed_phases: 3
  total_plans: 8
  completed_plans: 8
  percent: 100
---

# Project State

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-04-22)

**Core value:** One person can define, launch, and evolve multiple kinds of agents from the same harness, including a coding agent backed by OpenCode, without rewriting the orchestration layer each time or guessing what the first external protocol actually supports.

**Current focus:** Phase 4 (Run Visibility And Control)

**Session continuity:** Phase 3 was paused and handed off in `.planning/HANDOFF.json` and `.planning/phases/03-opencode-coding-lane/.continue-here.md` on 2026-04-22T20:50:05Z. Resume with `$gsd-resume-work` and continue from Phase 4.

## Notes

- The repo already contains a working agent runtime, session API, provider config, skills support, and workspace tools.
- The new work is a higher-level harness layer on top of that runtime, not a replacement for it.
- OpenCode is the first external coding backend to support.
- ACP discovery now has a validated subset documented in `docs/implementation/opencode-acp-capability-map.md`.
- Working terminology is still provisional.

**Completed Phase:** 3 (OpenCode Coding Lane) — 3/3 plans complete — 2026-04-22
**Next Phase:** 4 (Run Visibility And Control), focused on run observability and interruption controls.

## Decisions

- OpenCode bindings persist connection defaults separately from general profiles.
- OpenCode binding persistence supports both YAML files and SQL with restart-safe reuse.
- Launch path is constrained to initialize/session-new/session-prompt plus session/update handling only.
- Launcher resolves saved profile+binding first and emits typed launch error kinds for API mapping.
- Expose OpenCode binding CRUD and launch endpoints with deterministic problem-details mapping.
- Require OpenCode binding and launcher dependencies in runtime HTTP handler constructor.
- Keep launch requests selector-based (profileName/bindingName/prompt) with no full config payload re-entry.

## Last Session

- Completed 03-03-PLAN.md
- Created pause handoff for the end of Phase 3 before moving into Phase 4
