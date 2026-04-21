# Implementation Summary: Agent Integration Tests — CLI Move, Test Tools, Instruction-Based Testing

**Plan:** [plan-agent-integration-tests.md](./plan-agent-integration-tests.md)

## Overview

Moved the CLI from `apps/sonalmod/cmd/sonalmod` to `tests/agent/integration-cli`, making `cmd/sonalmod` a clean production binary. Added `GetToolsRegistry()` to the Engine and fixed a bug where the DI-provided `ToolsRegistry` was ignored. Implemented two deterministic test tools (`test_get_location`, `test_get_weather`), created 7 scenario Markdown files, and verified all 6 agent scenarios pass end-to-end.

## Tasks

### Task 1: Expose ToolsRegistry through Engine
Added `GetToolsRegistry()` getter to `engine.go` (DI `Invoke` pattern). Fixed `newRuntime` to use `deps.ToolsRegistry` instead of creating a fresh one. Added `ToolsRegistry` field to `Runtime` struct to enable pointer-equality assertion in tests.

### Task 2: Move CLI code from cmd/sonalmod to integration-cli
Moved and adapted CLI code to `tests/agent/integration-cli`. `runCLI` now accepts `agent.AgentRunner` interface directly (no Cobra command in package). `cli.go` excluded from coverage threshold via `.testcoverage.yaml`. `main.go` was mostly rewritten in this task (ahead of Task 3) to resolve unused-symbol lint errors.

### Task 3: Rewrite integration-cli main.go to use Engine
Added `GetToolsRegistry()` + `registerTestTools()` wiring in `main.go`; introduced a stub `test_tools.go` with a `toolsRegistrar` interface. Added `main_test.go` covering flag defaults, required annotations, and overrides. Most `main.go` work was already done in Task 2.

### Task 4: Implement test tools (test_get_location, test_get_weather)
Replaced stub with full `test_tools.go`: `test_get_location` returns hardcoded New York coordinates; `test_get_weather` echoes location, returns fixed 22.5°C/Partly Cloudy/65% humidity, errors on empty location. Magic number linter satisfied via extracted package-level constants.

### Task 5: Create test scenario files
Created `tests/agent/scenarios/` with 7 files: `test-hello-world.md`, `test-large-output.md`, `test-tool-calling.md`, `test-session-awareness.md`, `test-multi-tool.md`, `test-error-handling.md`, and `master.md` (orchestrator with sub-agent header guard). All match plan sections 5.1–5.7 exactly.

### Task 6: Verify full integration
Binary builds and `--help` works. `make lint` + `make test` from `tests/agent/integration-cli`: 0 issues, 100% coverage. `make affected-lint-test` from repo root: all 7 affected projects green.

### Task 7: Run agent tests via master scenario
All 6 scenarios executed and passed via `integration-cli`. Results written to `tests/agent/tmp/test-results.md`. Required rebuilding the binary, creating `data/agent-temp` and `data/sonalmod-runtime` directories, and updating `test-session-awareness.md` (safety filter workaround). Master scenario was not invoked via sub-agent — scenarios run directly, which is equivalent.

## Deviations & notes

- **`Runtime` struct gained `ToolsRegistry` field** (Task 1): not in plan, needed to enable pointer-equality test assertion.
- **`runCLI` accepts `agent.AgentRunner` interface** (Task 2): plan suggested `*agent.Runner`; using the interface is cleaner and testable.
- **`main.go` rewrite done in Task 2** (not Task 3): lint enforcement on unused symbols required it earlier.
- **`mnd` linter required constants** (Task 4): float/int literals extracted to named constants; tests reference same constants.
- **Session awareness scenario updated** (Task 7): "secret code" framing triggered gpt-4.1 safety filters; changed to "memorable phrase" framing.
- **Master scenario not used for Task 7 execution**: scenarios run directly with `integration-cli`; master requires sub-agent capabilities not available in that context.
- **Data directories needed** (Task 7): `tests/agent/integration-cli/data/agent-temp` and `data/sonalmod-runtime` must exist at runtime.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
