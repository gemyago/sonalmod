# Implementation Summary: CLI Agent Runner for E2E Testing

**Plan:** [plan-cli-agent-runner.md](./plan-cli-agent-runner.md)

## Overview

The CLI agent runner work adds streaming output helpers, a `cli` Cobra subcommand that runs the agent via DI (`Runtime.Runner`), and wiring plus integration tests. Streaming uses `ConsumeEventsAsStringSeq`; the command supports `--prompt`, `--session`, and CLI-specific log defaults applied from the root `PersistentPreRunE` so `internal.Setup` runs correctly for `cli`.

## Tasks

### Task 1: Add `streamAgentOutput` helper with tests

Implemented `streamAgentOutput` to stream string chunks from `ConsumeEventsAsStringSeq` to an `io.Writer`, with tests for chunk order and stream failure after partial output. The public API uses a small `streamTextResult` interface (instead of concrete `*agent.RunResult`) so tests avoid importing `runtime/internal` outside `runtime/…`. A writer-error subtest was added partly to satisfy per-file coverage.

### Task 2: Add `newCLICmd` command definition and flag tests

Implemented `newCLICmd` with prompt/session flags, DI, and run flow; uses `agent.NewRunParams(...).WithText(...)` because `main` cannot build `*internal.MessageContent` directly. `PrepareCLILogDefaultsIfNeeded` prepares CLI log defaults; coverage excludes for `cli.go` were used where noted until integration coverage landed in Task 3.

### Task 3: Wire CLI command into `setupCommands` and add integration test

Registered `newCLICmd` in `setupCommands`; moved `PrepareCLILogDefaultsIfNeeded` to the root `PersistentPreRunE` and removed duplicate `PersistentPreRunE` from `cli` so the setup chain reaches `internal.Setup`. Integration test runs `cli` with prompt and tolerates LLM/API errors (e.g. 401) while verifying wiring.

## Deviations & notes

- **Task 1:** `streamTextResult` interface vs concrete `RunResult` in API; extra writer-error and coverage-motivated test beyond the two cases named in the plan.
- **Task 2:** `NewRunParams` / `WithText` in `runtime/agent`; `.testcoverage.yaml` exclude for `cli.go` until Task 3; log-default hook ordering documented and completed in Task 3.
- **Task 3:** Fixed ordering bug: `cli` subcommand `PersistentPreRunE` had blocked root setup; resolved by root-only pre-run for log defaults and removing `cli`’s `PersistentPreRunE`.

## Completion

- Lint: ✓
- Type check: ✓ (Go compile / module checks via `make test`)
- Tests: ✓
