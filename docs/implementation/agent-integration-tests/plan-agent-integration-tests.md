# Plan: Agent Integration Tests — CLI Move, Test Tools, Instruction-Based Testing

## Introduction / Overview

CLI command (`cli` subcommand) currently live inside `cmd/sonalmod`. It no belong there — `cmd/sonalmod` is production binary, CLI is testing artifact. CLI must move to `tests/agent/integration-cli` where it belong.

Once CLI relocated, integration-cli need dummy tools (fake location, fake weather) for testing agent tool-calling. These tools inject into `ToolsRegistry` — which Engine must expose via getter.

Then, scenario-based agent tests get built. Each test scenario = one instruction file living at `tests/agent/scenarios/`. Master scenario orchestrate all tests via sub-agents for token efficiency. Test scenarios cover: basic hello-world, large output streaming, tool calling, and session awareness.

**Problem:** No proper agent integration tests exist. CLI lives in wrong place. No test tools. No scenario-based test harness.

**Goal:** Move CLI to integration-cli, add test tools, build scenario-based agent test suite with master orchestrator.

## Business Logic

1. **CLI relocation:** `cli.go`, `cli_output.go` and related code move from `apps/sonalmod/cmd/sonalmod/` to `tests/agent/integration-cli/`. Integration-cli become real Go binary that can run agent prompts via Engine. `cmd/sonalmod` keep only `start` and `user` subcommands.

2. **Engine ToolsRegistry exposure:** Engine need `GetToolsRegistry()` getter to retrieve existing `ToolsRegistry` from DI container. External consumers (like integration-cli) create Engine, get registry, add custom tools to it before running agent. Also fix existing bug where `newRuntime` ignore DI-provided `ToolsRegistry` and create fresh one instead — should use provided one and add built-in tools to it.

3. **Test tools:** Two dummy tools get registered in integration-cli only:
   - `test_get_location` — return random city/country/coordinates
   - `test_get_weather` — return random temperature/conditions for given location
   Both return deterministic-looking but hardcoded random values. Simple, no external deps.

4. **Scenario-based tests:** Markdown scenario files drive agent behaviour testing. Each scenario tell agent what to do and what to verify. Master scenario use sub-agents to run individual test scenarios and collect results.

5. **Session awareness:** Test must prove agent remember context across interactions within same session.

## High-Level Architecture

```
apps/sonalmod/
├── engine.go                    # MODIFIED: add GetToolsRegistry() getter
├── internal/
│   └── runtime.go               # MODIFIED: use provided ToolsRegistry instead of creating new one
└── cmd/sonalmod/
    ├── main.go                  # MODIFIED: remove cli subcommand
    ├── cli.go                   # DELETED (moved)
    ├── cli_output.go            # DELETED (moved)
    ├── cli_test.go              # DELETED (moved)
    └── cli_output_test.go       # DELETED (moved)

tests/agent/
├── scenarios/                       # NEW: test scenario instruction files
│   ├── master.md                    # Master orchestrator instruction
│   ├── test-hello-world.md          # Simple hello world test
│   ├── test-large-output.md         # Large output streaming test
│   ├── test-tool-calling.md         # Tool calling test
│   ├── test-session-awareness.md    # Session awareness test
│   ├── test-multi-tool.md           # Multiple tool calls in sequence
│   └── test-error-handling.md       # Error/edge case handling
└── integration-cli/
    ├── main.go                      # REWRITTEN: real CLI using Engine
    ├── cli.go                       # MOVED from cmd/sonalmod (adapted)
    ├── cli_output.go                # MOVED from cmd/sonalmod (adapted)
    ├── cli_test.go                  # MOVED from cmd/sonalmod (adapted)
    ├── cli_output_test.go           # MOVED from cmd/sonalmod (adapted)
    ├── test_tools.go                # NEW: dummy test tools
    ├── test_tools_test.go           # NEW: tests for dummy tools
    └── go.mod                       # MODIFIED: add sonalmod dependency

runtime/agent/
└── tools_registry.go            # NO CHANGES (already public API sufficient)
```

## Detailed Architecture

### Component 1: Engine ToolsRegistry Exposure (`apps/sonalmod/`)

**`engine.go`** — Add getter method to retrieve ToolsRegistry from DI container:
```go
func (e *Engine) GetToolsRegistry() (*agent.ToolsRegistry, error) {
    var reg *agent.ToolsRegistry
    err := e.container.Invoke(func(r *agent.ToolsRegistry) {
        reg = r
    })
    return reg, err
}
```

No new `EngineOpt` needed. External consumers call `engine.GetToolsRegistry()` after `NewEngine()`, add their tools, and those tools are available when `GetRuntime()` creates the runner.

**`internal/runtime.go`** — Fix bug: `newRuntime` currently ignore `deps.ToolsRegistry` and create fresh one. Change to:
```go
func newRuntime(deps RuntimeDeps) (*Runtime, error) {
    toolsRegistry := deps.ToolsRegistry  // Use DI-provided one
    // ... register built-in tools on it as before ...
}
```

This way: DI provide `*agent.ToolsRegistry` from `registerRuntime`. External consumer get same instance via `GetToolsRegistry()`, add custom tools. When `newRuntime` run, it use that same registry (with custom tools already added) and register built-in tools on top.

### Component 2: CLI Move to integration-cli

**Files to move** (from `apps/sonalmod/cmd/sonalmod/` to `tests/agent/integration-cli/`):
- `cli.go` → adapt: remove Cobra dependency if simpler, or keep Cobra. Use Engine directly instead of `newEngineFromRoot`. Import `sonalmod` package.
- `cli_output.go` → move as-is (standalone helper)
- `cli_test.go` → adapt imports
- `cli_output_test.go` → move as-is

**`main.go`** — Rewrite from stub to real binary:
```go
func main() {
    // Create Engine
    engine, err := sonalmod.NewEngine(
        sonalmod.WithEngineEnv("test"),
        // ... other opts from flags ...
    )

    // Get ToolsRegistry from DI and register test tools
    toolsRegistry, _ := engine.GetToolsRegistry()
    registerTestTools(toolsRegistry)

    // Run CLI (prompt/session from flags)
    rt, _ := engine.GetRuntime()
    runCLI(ctx, rt, prompt, sessionID, os.Stdout)
}
```

**`apps/sonalmod/cmd/sonalmod/main.go`** — Remove CLI subcommand from `setupCommands`. Remove `setPerCommandDefaults` logic for `cli`. Keep `start` and `user` only. Remove `engine_cmd.go` (if only used by CLI) or keep if still used by `start`/`user`.

### Component 3: Test Tools (`tests/agent/integration-cli/`)

**`test_tools.go`:**

```go
// test_get_location — return hardcoded random location
type locationResult struct {
    City      string  `json:"city"`
    Country   string  `json:"country"`
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
}

// test_get_weather — return hardcoded random weather for given location
type weatherInput struct {
    Location string `json:"location"`
}
type weatherResult struct {
    Location    string  `json:"location"`
    Temperature float64 `json:"temperature"`
    Unit        string  `json:"unit"`
    Conditions  string  `json:"conditions"`
    Humidity    int     `json:"humidity"`
}
```

Both tools use `agent.NewToolDef[TArgs, TResults]()` and get added via `registry.AddTools()`.

`test_get_location` take no input, return fixed location (e.g. "New York", "US", 40.7128, -74.0060).
`test_get_weather` take location string, return fixed weather (e.g. 22°C, "Partly Cloudy", 65% humidity).

Values hardcoded (not truly random) so tests can assert on them.

### Component 4: Scenario-Based Test Files

Scenarios live in `tests/agent/scenarios/`. Each scenario is Markdown that tell agent what to do and what success look like. Scenarios live at agent level (sibling to integration-cli), not inside integration-cli — they are test specifications that could be used by different test runners.

**`master.md`** — Master orchestrator:
- Run each test scenario as sub-agent
- Collect pass/fail results
- Report summary at end
- Write results to `tests/agent/tmp/` folder (create if not exist)
- Use sub-agents for token efficiency (each test get fresh context)
- **Header guard:** If agent has no sub-agent capabilities, report to user and exit immediately (do not attempt to run tests without sub-agents)

**`test-hello-world.md`** — Simple test:
- Instruction: "Respond with exactly: HELLO_WORLD_OK"
- Success: output contain "HELLO_WORLD_OK"

**`test-large-output.md`** — Streaming test:
- Instruction: "Generate a numbered list from 1 to 200, each on new line"
- Success: output contain all 200 numbers, verify streaming work for large output

**`test-tool-calling.md`** — Tool calling test:
- Instruction: "Use test_get_location tool to find current location, then use test_get_weather tool to get weather for that location. Report both results."
- Success: output contain location data AND weather data from tools

**`test-session-awareness.md`** — Session memory test:
- Instruction (interaction 1): "Remember this secret code: ZEBRA_42_ALPHA. Confirm you stored it."
- Instruction (interaction 2, same session): "What was the secret code I told you earlier?"
- Success: second interaction output contain "ZEBRA_42_ALPHA"

**`test-multi-tool.md`** — Multiple sequential tool calls:
- Instruction: "Call test_get_location twice and compare results. Are they same?"
- Success: agent call tool twice, report results

**`test-error-handling.md`** — Edge case:
- Instruction: "Call test_get_weather with empty location. Report what happens."
- Success: agent handle gracefully, no crash

## Key Architectural Decisions

1. **CLI move, not copy.** CLI code physically move from `cmd/sonalmod` to `integration-cli`. No duplication. `cmd/sonalmod` become cleaner production binary.

2. **Engine getter, not injection.** Use `GetToolsRegistry()` getter on Engine to retrieve existing DI-provided registry. External consumers add tools to it before triggering runtime creation via `GetRuntime()`. Simpler than injection — no new options, no DI override complexity.

3. **Fix `newRuntime` bug.** Currently `deps.ToolsRegistry` ignored. Fix make DI-provided registry actually used. Built-in tools (workspacefs, skills) still register on whatever registry is provided.

4. **Hardcoded tool values, not random.** Test tools return deterministic values so assertions possible. "Random" in user request interpreted as "dummy/fake" not literally `rand`.

5. **Scenario files = test specs.** Markdown scenarios serve as both test specification and agent prompt. Machine-readable success criteria in each file. Live at `tests/agent/scenarios/` level — sibling to integration-cli, not inside it.

6. **Sub-agent pattern for master scenario.** Master scenario spawn sub-agents per test case. Each sub-agent get clean context — save tokens, isolate failures.

7. **integration-cli stay separate Go module.** Keep existing module boundary. Add dependency on `apps/sonalmod` (the `sonalmod` package) and `runtime/agent` via `go.work`.

## Uncertainties

1. **Cobra vs simple flags:** Should integration-cli keep Cobra (from moved CLI code) or simplify to plain `flag` package? Cobra add weight but keep consistency. Plan assume keep Cobra for now since code already written with it.

2. **Master scenario execution:** How master scenario actually invoke sub-agents depend on agent capabilities. Master instruction must include header guard: if no sub-agent capabilities detected, report to user and exit. Do not attempt to run tests without sub-agents.

3. **Session awareness test mechanics:** Test need two sequential runs with same session ID. Integration-cli already support `--session` flag. Test harness (the outer agent from tests/AGENTS.md) must handle running CLI twice and passing session ID between runs.

4. **Test tool error behaviour:** `test_get_weather` with empty location — should it return error or empty result? Plan assume return error for better edge case testing.

5. **`engine_cmd.go` fate:** `newEngineFromRoot` currently used by both `start` and `cli` commands. After CLI moves out, check if `start` and `user` commands still need it. If yes, keep. If only CLI used it, it move too.

## Related Files

### Files to modify
- `apps/sonalmod/engine.go` — add `GetToolsRegistry()` getter
- `apps/sonalmod/internal/runtime.go` — use provided ToolsRegistry instead of creating new
- `apps/sonalmod/cmd/sonalmod/main.go` — remove CLI subcommand
- `apps/sonalmod/cmd/sonalmod/main_test.go` — remove CLI-related tests
- `tests/agent/integration-cli/main.go` — rewrite to real CLI
- `tests/agent/integration-cli/go.mod` — add sonalmod + runtime deps

### Files to delete (from cmd/sonalmod)
- `apps/sonalmod/cmd/sonalmod/cli.go`
- `apps/sonalmod/cmd/sonalmod/cli_output.go`
- `apps/sonalmod/cmd/sonalmod/cli_test.go`
- `apps/sonalmod/cmd/sonalmod/cli_output_test.go`

### New files to create
- `tests/agent/integration-cli/cli.go` — moved+adapted CLI logic
- `tests/agent/integration-cli/cli_output.go` — moved streaming helper
- `tests/agent/integration-cli/cli_test.go` — moved+adapted tests
- `tests/agent/integration-cli/cli_output_test.go` — moved tests
- `tests/agent/integration-cli/test_tools.go` — dummy test tools
- `tests/agent/integration-cli/test_tools_test.go` — test tool tests
- `tests/agent/scenarios/master.md`
- `tests/agent/scenarios/test-hello-world.md`
- `tests/agent/scenarios/test-large-output.md`
- `tests/agent/scenarios/test-tool-calling.md`
- `tests/agent/scenarios/test-session-awareness.md`
- `tests/agent/scenarios/test-multi-tool.md`
- `tests/agent/scenarios/test-error-handling.md`

### Reference files (read-only)
- `runtime/agent/tools_registry.go` — ToolsRegistry, AddTools, DefinedTool
- `runtime/agent/tool.go` — ToolDef generic type, NewToolDef
- `runtime/agent/runner.go` — Runner, WithToolsRegistry
- `apps/sonalmod/internal/wireup.go` — Setup, registerRuntime
- `apps/sonalmod/cmd/sonalmod/engine_cmd.go` — newEngineFromRoot (may move or stay)
- `tools/workspacefs/tools.go` — reference for tool registration pattern
- `tools/skills/tools.go` — reference for tool registration pattern

## Task List

TDD approach must be followed. Module-specific task completion protocol must be followed after each task.

---

**Task 1: Expose ToolsRegistry through Engine**
- Add `GetToolsRegistry() (*agent.ToolsRegistry, error)` getter to `engine.go` (resolve from DI container, same pattern as `GetAgentRunner`)
- Fix `internal/runtime.go` `newRuntime`: use `deps.ToolsRegistry` instead of creating new `agent.NewToolsRegistry()`
- Write failing tests:
  - `engine_test.go`: verify `GetToolsRegistry()` returns non-nil registry from DI
  - `internal/runtime_test.go`: verify `newRuntime` uses provided ToolsRegistry (tools from provided registry present in runner)
- Run affected tests: `go test -v ./... --run <test pattern>` from `apps/sonalmod`
  - Verify failure is expectation-based (not compilation errors)
- Implement the changes
- Run affected tests again, verify pass
- Run `make lint` and `make test` from `apps/sonalmod`
- Write summary to `doc/implementation/agent-integration-tests/summary-task-1.md`

---

**Task 2: Move CLI code from cmd/sonalmod to integration-cli**
- Copy `cli.go`, `cli_output.go`, `cli_test.go`, `cli_output_test.go` from `apps/sonalmod/cmd/sonalmod/` to `tests/agent/integration-cli/`
- Adapt imports: change package references from internal sonalmod paths to public `sonalmod` package imports
- Adapt `cli.go`:
  - Remove dependency on `newEngineFromRoot` (Engine created in `main.go` instead)
  - Accept `*agent.Runner` or `*internal.Runtime` directly as parameter (instead of resolving from DI container in CLI command)
  - Keep Cobra command structure for `--prompt` and `--session` flags
- Update `tests/agent/integration-cli/go.mod`: add dependencies on `github.com/gemyago/sonalmod/apps/sonalmod` and `github.com/gemyago/sonalmod/runtime`
- Delete original files from `apps/sonalmod/cmd/sonalmod/`: `cli.go`, `cli_output.go`, `cli_test.go`, `cli_output_test.go`
- Update `apps/sonalmod/cmd/sonalmod/main.go`: remove `newCLICmd` from `setupCommands`, remove `setPerCommandDefaults` CLI-specific logic
- Update `apps/sonalmod/cmd/sonalmod/main_test.go`: remove CLI-related test cases
- Evaluate `engine_cmd.go`: if `newEngineFromRoot` still used by `start`/`user`, keep; otherwise move/delete
- Run `make lint` and `make test` from `apps/sonalmod` — verify no breakage
- Run `make lint` and `make test` from `tests/agent/integration-cli` — verify moved code compile and tests pass
- Write summary to `doc/implementation/agent-integration-tests/summary-task-2.md`

---

**Task 3: Rewrite integration-cli main.go to use Engine**
- Rewrite `tests/agent/integration-cli/main.go`:
  - Parse CLI flags (--prompt, --session, --env, --log-level, --logs-file, --json-logs)
  - Create Engine: `sonalmod.NewEngine(...)`
  - Get ToolsRegistry from Engine: `engine.GetToolsRegistry()`
  - Call `registerTestTools(toolsRegistry)` (stub for now, implemented in Task 4)
  - Get Runtime: `engine.GetRuntime()`
  - Call `runCLI(ctx, rt, prompt, sessionID, os.Stdout)`
- Write failing tests:
  - Verify main wiring: Engine creates with test tools registry
  - Verify CLI flags parsed correctly
- Run affected tests: `go test -v ./... --run <test pattern>` from `tests/agent/integration-cli`
  - Verify failure is expectation-based
- Implement the main.go rewrite
- Run affected tests, verify pass
- Run `make lint` and `make test` from `tests/agent/integration-cli`
- Write summary to `doc/implementation/agent-integration-tests/summary-task-3.md`

---

**Task 4: Implement test tools (test_get_location, test_get_weather)**
- Create `tests/agent/integration-cli/test_tools.go`:
  - Define `ToolsRegistry` interface (consumer-defined, just `AddTools`)
  - Define `registerTestTools(registry ToolsRegistry)` function
  - Implement `test_get_location` tool:
    - No input args (use empty struct)
    - Return `locationResult{City: "New York", Country: "US", Latitude: 40.7128, Longitude: -74.0060}`
  - Implement `test_get_weather` tool:
    - Input: `weatherInput{Location string}`
    - Return `weatherResult{Location: input.Location, Temperature: 22.5, Unit: "celsius", Conditions: "Partly Cloudy", Humidity: 65}`
    - If Location empty, return error
  - Use `agent.NewToolDef[TArgs, TResults]()` pattern from `tools/workspacefs`
- Create `tests/agent/integration-cli/test_tools_test.go`:
  - Write failing tests first:
    - `test_get_location` return expected location data
    - `test_get_weather` return expected weather for given location
    - `test_get_weather` with empty location return error
    - `registerTestTools` add exactly 2 tools to registry
  - Run affected tests: `go test -v ./... --run <test pattern>`
    - Verify failure is expectation-based (not compilation errors)
  - Implement tool logic
  - Run affected tests, verify pass
- Run `make lint` and `make test` from `tests/agent/integration-cli`
- Write summary to `doc/implementation/agent-integration-tests/summary-task-4.md`

---

**Task 5: Create test scenario files**
- Create `tests/agent/scenarios/` directory
- Create instruction files:

**Task 5.1: test-hello-world.md**
- Instruction: ask agent respond with exact string "HELLO_WORLD_OK"
- Success criteria: output contain "HELLO_WORLD_OK"

**Task 5.2: test-large-output.md**
- Instruction: ask agent generate numbered list 1 to 200
- Success criteria: output contain numbers 1-200 on separate lines, verify streaming handle large output

**Task 5.3: test-tool-calling.md**
- Instruction: ask agent use `test_get_location` then `test_get_weather` for that location
- Success criteria: output contain location (New York) and weather data (22.5, Partly Cloudy)

**Task 5.4: test-session-awareness.md**
- Instruction part 1: "Remember secret code: ZEBRA_42_ALPHA"
- Instruction part 2 (same session): "What was the secret code?"
- Success criteria: second output contain "ZEBRA_42_ALPHA"
- Note: this test require two sequential CLI invocations with same session ID

**Task 5.5: test-multi-tool.md**
- Instruction: call `test_get_location` twice, report if results are same
- Success criteria: agent call tool twice, both results present in output

**Task 5.6: test-error-handling.md**
- Instruction: call `test_get_weather` with empty string location
- Success criteria: agent handle error gracefully, report what happened

**Task 5.7: master.md**
- Master orchestrator scenario
- Header guard: if no sub-agent capabilities, report to user and exit
- For each test scenario: spawn sub-agent, run test, collect result
- Report summary: test name + PASS/FAIL + brief reason
- Write results to `tests/agent/tmp/` folder (create if not exist)
- Use sub-agents for token efficiency

- No lint/test required (markdown files only)
- Write summary to `doc/implementation/agent-integration-tests/summary-task-5.md`

---

**Task 6: Verify full integration**
- Build integration-cli binary: `go build -o ./bin/integration-cli ./` from `tests/agent/integration-cli`
- Verify binary runs with `--help`
- Run `make lint` and `make test` from `tests/agent/integration-cli`
- Run `make affected-lint-test` from repo root — verify no breakage across all modules
- Write summary to `doc/implementation/agent-integration-tests/summary-task-6.md`

---

**Task 7: Run agent tests via master scenario**
- Execute master scenario using integration-cli
- Verify all test scenarios pass (hello-world, large-output, tool-calling, session-awareness, multi-tool, error-handling)
- Check results written to `tests/agent/tmp/`
- Fix any failing scenarios
- Write summary to `doc/implementation/agent-integration-tests/summary-task-7.md`

---

**Task 8: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
