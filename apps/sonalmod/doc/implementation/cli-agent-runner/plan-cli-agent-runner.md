# Plan: CLI Agent Runner for E2E Testing

## Introduction / Overview

The sonalmod application currently exposes the agent runner only through its HTTP API. For end-to-end testing scenarios, we need a way to invoke the agent runner directly from the command line—sending a prompt, receiving streamed output, and optionally continuing a previous session. This feature ports the relevant aspects of the community-manager `llmagent` CLI into the sonalmod application as a `cli` sub-command, reusing the `*agent.Runner` that is already wired through DI.

**Problem:** There is no way to run the agent from a terminal for quick e2e smoke tests without standing up the HTTP server and making HTTP requests.

**Goal:** Add a `sonalmod cli` sub-command that accepts `--prompt`, optionally `--session`, streams the agent response to stdout, and can be used in scripted e2e test pipelines.

## Business Logic

1. **Prompt execution:** The user provides a text prompt via `--prompt` flag. The CLI sends it to the agent runner already available in the DI container and streams the response to stdout.
2. **Session continuity:** An optional `--session` flag allows reusing an existing session ID. When omitted, a new UUID is generated. This enables multi-turn conversation testing.
3. **Output:** The agent's text response is streamed directly to stdout (always streaming mode). All logs go to a log file (`--logs-file` defaults to `sonalmod-cli.log`; `--json-logs` defaults to `true` for the CLI command), keeping stdout clean for piping/assertions.
4. **Session ID on exit:** After the agent output completes, a newline and the session ID are printed to stdout so the user can easily copy it for `--session` on the next invocation.

## High-Level Architecture

```
main.go (setupCommands)
  ├── rootCmd (sonalmod)
  │     └── PersistentPreRunE → internal.Setup (existing)
  ├── startCmd (start) ← existing
  └── cliCmd (cli) ← NEW
        ├── Flags: --prompt, --session
        ├── Defaults: --logs-file=sonalmod-cli.log, --json-logs=true
        ├── PreRunE: no additional DI registration needed (Runtime is already in the container)
        └── RunE: container.Invoke → run agent via Runtime.Runner → stream output to stdout → print session ID
```

The key insight: `internal.Setup` already registers `*internal.Runtime` (which holds `*agent.Runner`) in the DI container. The new `cli` command simply invokes `Runtime.Runner.Run(...)` with the user's prompt—no additional infrastructure wiring is required.

### Components

1. **`cli.go`** (new file in `apps/sonalmod/`) — defines the `newCLICmd` cobra command with prompt/session flags and CLI-specific defaults.
2. **`cli_output.go`** (new file in `apps/sonalmod/`) — `streamAgentOutput` function that streams `RunResult` events to an `io.Writer`.
3. **`main.go`** (existing) — updated `setupCommands` to add the `cli` sub-command.

## Detailed Architecture

### `cli.go` — The CLI sub-command

Location: `apps/sonalmod/cli.go`

```go
type cliDeps struct {
    dig.In
    Runtime    *internal.Runtime
    RootLogger *slog.Logger
}
```

The `newCLICmd(container)` function creates a cobra command with:
- `--prompt` / `-p` (required string) — the user prompt
- `--session` / `-s` (optional string) — session ID to reuse

CLI-specific defaults (set in `PreRunE` before `internal.Setup` or via command flag defaults):
- `--logs-file` defaults to `"sonalmod-cli.log"` (overrides root's empty default)
- `--json-logs` defaults to `true` (overrides root's `false` default)

`RunE` implementation:
1. Generate a session ID if none provided (via `uuid.New().String()`)
2. Build `agent.RunParams` with the prompt as a `MessageContent` with a single text part
3. Call `deps.Runtime.Runner.Run(ctx, params)`
4. Stream the result to `os.Stdout` via `streamAgentOutput`
5. Print `"\n"` + session ID to stdout so the caller can capture it for session restore

The `RunParams` type is `internal.RunParams` (aliased in `runtime/agent` as `agent.RunParams`). It expects:
- `UserID` — hardcoded to `"cli-user"` for CLI invocations
- `SessionID` — from flag or generated UUID
- `Message` — `*internal.MessageContent` with text parts

### `cli_output.go` — Streaming output helper

Location: `apps/sonalmod/cli_output.go`

Port of the community-manager's `streamAgentOutput` function, adapted for the sonalmod `RunResult` type. Always streams (no non-streaming mode):

- Iterates `result.ConsumeEventsAsStringSeq(ctx)`, writing each chunk to the writer immediately via `fmt.Fprint`.

### `main.go` — Registration

Add `newCLICmd(container)` to `setupCommands`:

```go
func setupCommands() *cobra.Command {
    container := dig.New()
    rootCmd := newRootCmd(container)
    rootCmd.AddCommand(
        newStartServerCmd(container),
        newCLICmd(container),        // NEW
    )
    return rootCmd
}
```

## Key Architectural Decisions

1. **Reuse existing DI setup.** The `cli` command relies on the same `PersistentPreRunE` → `internal.Setup` path as `start`. This means `*internal.Runtime` (and therefore `*agent.Runner`) is already in the container. No separate wiring function is needed (unlike the community-manager which had its own `wireupRootDeps`).

2. **No separate binary or `cmd/` directory.** The AGENTS.md for sonalmod states: "Consumer-facing entrypoint is `main.go` at the module root (no additional command binaries under `cmd/`)." The CLI command is a sub-command of the existing `sonalmod` binary.

3. **Hardcoded CLI user ID.** Since the CLI is for e2e testing, the user ID is a constant (`"cli-user"`), similar to the community-manager's `pocAgentUserID`.

4. **Always streaming, no `--streaming` flag.** Streaming is the only output mode. A non-streaming mode adds complexity with no benefit for the e2e testing use case.

5. **CLI-specific log defaults.** The `cli` sub-command overrides the root defaults: `--logs-file` defaults to `"sonalmod-cli.log"` and `--json-logs` defaults to `true`. This keeps stdout clean for agent output and piping/assertions without the user having to remember extra flags.

6. **Session ID printed after output.** After the streamed agent response, a newline followed by the session ID is printed to stdout. This makes it trivial to grab the session ID from the last line of output and pass it back via `--session` for multi-turn testing.

7. **No interactive/REPL mode in initial implementation.** The community-manager has an `interactive` command with TTY/pipe mode detection. This is out of scope for the initial implementation. The `--prompt` mode with `--session` restore is sufficient for e2e testing. Interactive mode can be added later if needed.

8. **No e2e-specific mock tools.** The community-manager registers fake weather/location/time tools for its e2e. We do not port those. The CLI will use whatever tools are configured on the runner (currently none in the default `NewRuntime`, but can be extended later).

9. **File placement.** New files live alongside `main.go` in `apps/sonalmod/` (package `main`), following the existing pattern where `main.go` already contains `newStartServerCmd` in the same package.

## Uncertainties

1. **Session persistence across CLI invocations.** The current `agent.NewRunner` uses `session.InMemoryService()`, which means sessions are lost between process runs. The `--session` flag is still useful within a single process lifetime (e.g., if we later add interactive mode), but for cross-invocation session restore, a persistent session service would be needed. This is a known limitation for now.

2. **Tool registration for CLI.** The current `NewRuntime` does not call `WithToolsRegistry`. If e2e tests need tools, the CLI command may need to register them. This can be addressed in a follow-up.

## Related Files

### Existing files to modify
- `apps/sonalmod/main.go` — add `newCLICmd` to `setupCommands`
- `apps/sonalmod/main_test.go` — add tests for the `cli` sub-command

### New files to create
- `apps/sonalmod/cli.go` — CLI sub-command definition and agent execution
- `apps/sonalmod/cli_output.go` — streaming output helper
- `apps/sonalmod/cli_test.go` — tests for CLI command flags
- `apps/sonalmod/cli_output_test.go` — tests for `streamAgentOutput`

### Reference files (read-only context)
- `apps/sonalmod/internal/runtime.go` — `Runtime` struct and `NewRuntime`
- `apps/sonalmod/internal/wireup.go` — `Setup` function (already registers Runtime)
- `runtime/agent/runner.go` — `Runner.Run`, `RunParams`, `RunResult`
- `runtime/internal/run_result.go` — `ConsumeEventsAsStringSeq`
- `runtime/internal/message_content.go` — `MessageContent`, `MessagePart`
- `runtime/internal/agentrun.go` — `RunParams` definition
- `tmp/community-manager/cmd/llmagent/root.go` — reference CLI implementation
- `tmp/community-manager/cmd/llmagent/run.go` — reference agent execution
- `tmp/community-manager/cmd/llmagent/root_test.go` — reference test patterns

## Task List

TDD approach must be followed. Module-specific task completion protocol must be followed after each task.

---

**Task 1: Add `streamAgentOutput` helper with tests**
- Create `apps/sonalmod/cli_output.go` with `streamAgentOutput(ctx, output, result)` function
  - Iterate `result.ConsumeEventsAsStringSeq(ctx)`, write each chunk to `output` via `fmt.Fprint`
- Create `apps/sonalmod/cli_output_test.go` with `TestStreamAgentOutput`
  - Write failing tests first:
    - writes chunks in order
    - propagates stream error while preserving partial output
  - Run affected tests: `go test -v ./... --run TestStreamAgentOutput`
    - Verify failure is expectation-based (not compilation errors)
  - Implement the `streamAgentOutput` function
  - Run affected tests again and verify all pass
- Run `make lint` and `make test` from `apps/sonalmod`
- Write summary to `doc/implementation/cli-agent-runner/summary-task-1.md`

---

**Task 2: Add `newCLICmd` command definition and flag tests**
- Create `apps/sonalmod/cli.go` with:
  - `cliDeps` struct (with `dig.In`, `*internal.Runtime`, `*slog.Logger`)
  - `cliUserID` constant
  - `newCLICmd(container *dig.Container) *cobra.Command` with flags `--prompt`/`-p` (required), `--session`/`-s` (optional)
  - CLI-specific defaults: `--logs-file` → `"sonalmod-cli.log"`, `--json-logs` → `true` (set via `PersistentPreRunE` or flag defaults on the sub-command, only when not explicitly provided by user)
  - `RunE` implementation:
    - Generate UUID for session if empty
    - Build `agent.RunParams{UserID: cliUserID, SessionID: sessionID, Message: ...}` with a single text `MessagePart`
    - Call `deps.Runtime.Runner.Run(ctx, params)`
    - Call `streamAgentOutput(ctx, os.Stdout, result)`
    - Print `"\n"` + session ID to stdout
- Create `apps/sonalmod/cli_test.go` with `TestNewCLICmd`:
  - Write failing tests first:
    - has `--prompt` flag marked as required
    - has `--session` flag with empty default
    - command Use is `"cli"`
  - Run affected tests: `go test -v ./... --run TestNewCLICmd`
    - Verify failure is expectation-based
  - Implement `newCLICmd`
  - Run affected tests and verify all pass
- Run `make lint` and `make test` from `apps/sonalmod`
- Write summary to `doc/implementation/cli-agent-runner/summary-task-2.md`

---

**Task 3: Wire CLI command into `setupCommands` and add integration test**
- Update `apps/sonalmod/main.go`:
  - Add `newCLICmd(container)` to `setupCommands`
- Update `apps/sonalmod/main_test.go`:
  - Write failing test first:
    - `cli` sub-command: verify the command executes through DI setup with `--prompt "test" -e test --logs-file ../../test.log`
    - Note: this will attempt a real agent run which will fail without LLM credentials in test env. The test should verify DI setup succeeds (command does not error on flag parsing / container wiring). The actual agent call error is acceptable in unit tests — or mock at the `Runtime` level if feasible.
  - Run affected tests: `go test -v ./... --run TestMain`
    - Verify failure is expectation-based
  - Implement the wiring in `setupCommands`
  - Run affected tests and verify behavior is as expected
- Run `make lint` and `make test` from `apps/sonalmod`
- Write summary to `doc/implementation/cli-agent-runner/summary-task-3.md`

---

**Task 4: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
