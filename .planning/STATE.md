---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: unknown
last_updated: "2026-04-22T18:32:44.819Z"
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 5
  completed_plans: 3
  percent: 60
---

# Project State

## Project Reference

See: `.planning/PROJECT.md` (updated 2026-04-22)

**Core value:** One person can define, launch, and evolve multiple kinds of agents from the same harness, including a coding agent backed by OpenCode, without rewriting the orchestration layer each time or guessing what the first external protocol actually supports.

**Current focus:** Phase --phase — 02

## Notes

- The repo already contains a working agent runtime, session API, provider config, skills support, and workspace tools.
- The new work is a higher-level harness layer on top of that runtime, not a replacement for it.
- OpenCode is the first external coding backend to support.
- ACP discovery now has a validated subset documented in `docs/implementation/opencode-acp-capability-map.md`.
- Working terminology is still provisional.

**Completed Phase:** 1 (ACP Discovery And Capability Map) — 2/2 plans complete — 2026-04-22
**Next Phase:** 2 (Agent Profile Foundation), using the validated ACP subset from Phase 1.
