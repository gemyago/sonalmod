# Implementation Summary: Add Skills Support to Sonalmod Runtime

**Plan:** [plan-skills-support.md](./plan-skills-support.md)

## Overview

Implemented first-class skills support for the Sonalmod runtime using a hybrid progressive-disclosure model: a new `tools/skills` module handles discovery, parsing, and on-demand full skill loading; the runtime `WithInstructionSuffix` hook injects a compact `<available_skills>` metadata block into the agent instruction; and the `apps/sonalmod` config layer wires everything together with an opt-in `skills.enabled` flag. All phases followed TDD, with lint and tests green across all touched modules.

## Tasks

### Task 1.1: Scaffold `tools/skills` module and registration contract (TDD)
Added the `tools/skills` Go module (layout mirroring `tools/workspacefs`), registration API with options, stub tools and tests, `go.work` entry, and `AGENTS.md`. Catalog and parser wiring was intentionally deferred to Task 1.2.

### Task 1.2: Implement skill parser and catalog discovery service (TDD)
Implemented YAML frontmatter parsing for `SKILL.md` (required/optional fields and validation) and a `Catalog` service that discovers skills under roots with size/entry limits, duplicate-name rules, and nil-safe `List`/`Get`; overall coverage 93.7%.

### Task 1.3: Implement `skills_list` and `skills_read` tools (TDD)
Wired `iskills.Catalog` into `skills_list` and `skills_read` handlers (metadata-only list, full read with path-safe errors for unknown skills) and updated `RegisterTools` to construct the catalog after path validation.

### Task 2.1: Add runner instruction augmentation support (TDD)
Added `WithInstructionSuffix` runner option and `Runner.buildInstruction()` so callers can append a non-empty suffix to `defaultRunnerAgentInstruction`; empty suffix leaves base instruction unchanged.

### Task 2.2: Wire skills config and tool registration in app runtime (TDD)
Added a `skills` config block in sonalmod, wired `RuntimeDeps` and `NewRuntime` so a skills catalog, skill tools, and an instruction suffix are applied only when skills are enabled; extended `tools/skills` with catalog helpers and edge-case tests.

### Task 3.1: Add integration/regression coverage for skills flow (TDD)
Added `TestSkillsFlowRegression` covering instruction fragments from `BuildSystemPromptFragments`, `skills_read` content fidelity, disabled-skills isolation (no skill tools), and startup-only catalog semantics.

### Task 3.2: Update docs and module guidance
Updated `tools/skills/AGENTS.md`, `runtime/AGENTS.md`, and `apps/sonalmod/AGENTS.md` to document skills registration patterns, `WithInstructionSuffix`, and `skills` config keys. No code changes required.

## Deviations & notes

- **Task 1.1:** `internal/skills` package kept empty (service/parser deferred to 1.2); path validation deferred to 1.2 catalog service (scaffold only rejects empty/nil paths).
- **Task 1.2:** `gopkg.in/yaml.v3` added as direct dependency; `go mod tidy` required workspace network resolution. Some OS-level error branches left uncovered without mocks (coverage still 93.7%).

## Completion

- Lint: ✓
- Tests: ✓
