# Implementation Summary: Add shell execution capabilities to `workspacefs`

**Plan:** [plan-exec-tool.md](./plan-exec-tool.md)

## Overview

`workspacefs` gained an opt-in exec tool family (`workspacefs_exec_command`, `workspacefs_exec_job_output`, `workspacefs_exec_kill_job`) with registration options, validation, and conservative defaults. Foreground execution uses bounded output and timeouts; background jobs use a service-scoped registry with polling, kill, and cleanup on `Service.Close`. Command policy (denylist) and `apps/sonalmod` runtime config wire-through complete the stack; module docs describe the security boundary between `os.Root` file access and shell execution.

## Tasks

### Task 1.1: Add exec registration contract and safe defaults

Introduced `ExecOptions`, `WithExec`, validation, defaults, `ExecConfig`/`WithExecEnabled`, stub exec service methods and agent tool wiring, plus tests. Exec is opt-in; enabling it adds three tools (12 total). Stub methods in `exec.go` use `context.Context`; handlers pass `tc`, which embeds `context.Context`, matching other tools. Early `exec_test.go` stubs were coverage-oriented placeholders superseded in 1.2.

### Task 1.2: Implement foreground command execution in service (TDD)

Implemented `ExecCommand` with workspace/workdir/command validation, shell wrapping, default timeout, bounded stdout/stderr, exit codes and timeout handling; refactored workspace picking via `pickWorkspaceEntry` for absolute paths. `TimedOut` uses a single post-`Run()` `DeadlineExceeded` check. Windows `buildShellCommand` is not covered on non-Windows CI; acceptable for cross-platform code. `ExecJobOutput` / `ExecKillJob` remained stubs until 2.1.

### Task 2.1: Implement background jobs, polling, and kill APIs (TDD)

Background execution uses a UUID-keyed job registry, polling (`ExecJobOutput`), kill (`ExecKillJob`), max concurrent jobs, and `Service.Close` cleanup; `ExecCommand` dispatches foreground vs background. Background jobs use `context.WithTimeout(context.Background(), ...)` and do not inherit the caller’s request context. Job IDs are service-global; poll requests validate workspace but do not scope lookup by workspace. `cmd.Start()` failure in `startBackgroundJob` is hard to test reliably.

### Task 2.2: Register and validate new tool definitions (TDD)

Tests in `agent_tools_test.go` cover the three exec tools, model-visible errors, and JSON round-trips. Wiring in `agent_tools.go` had already landed in earlier tasks, so this task was mostly test-focused (no strict red→green cycle for wiring alone).

### Task 3.1: Add command policy restrictions and hardening (TDD)

Added `checkCommandPolicy` (configurable denylist or built-in high-risk patterns), integrated after workspace/workdir checks, with tests for policy and path-leak behavior including background mode.

### Task 3.2: Wire runtime config and registration options (TDD)

YAML and DI expose workspacefs exec settings; `NewRuntime` passes `WithExec` only when enabled, using zero values so workspacefs applies built-in defaults where appropriate.

### Task 4.1: Update module docs and finalize behavior docs

Expanded `tools/workspacefs/AGENTS.md` with exec contract, tools, limits, denylist, job lifecycle, and the filesystem vs command-execution boundary.

## Deviations & notes

| Area | Note |
|------|------|
| 1.1 / 1.2 | Early stub tests and timeout detection shape differ slightly from initial sketches; foreground behavior fully covered in 1.2. |
| 2.1 | Background contexts and global job IDs are intentional; poll workspace validation is for API consistency. |
| 2.2 | Tests added after wiring existed from prior tasks. |
| Cross-platform | Windows shell path not exercised on macOS/Linux runners. |

## Completion

- Lint: ✓
- Type check: N/A (Go module)
- Tests: ✓
