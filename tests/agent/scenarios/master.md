# Master Test Orchestrator

## Purpose

Schedule the agent integration suite, collect PASS/FAIL from sub-agents, and write a summary report. **Orchestration only** — you do not execute test logic and you do not need to understand what each scenario verifies.

## Header Guard — Sub-Agent Capability Check

**STOP HERE if you do not have sub-agent capabilities.**

Before proceeding, verify you can spawn sub-agents. If you cannot:

1. Report to the user: `ERROR: Master scenario requires sub-agent capabilities. This agent cannot spawn sub-agents. Aborting test run.`
2. Exit immediately. Do not attempt to run any tests.

Only continue if sub-agent spawning is available.

## What the master MUST NOT do

Violations invalidate the run:

- **Do not** open, read, summarize, or quote the contents of any `tests/agent/scenarios/test-*.md` file. You do not need those instructions to orchestrate.
- **Do not** run `go run` (or any integration-cli commands) for scenario workloads in this **master** context. Sub-agents run the CLI.
- **Do not** put multiple scenario files into one sub-agent task. **One sub-agent per scenario file** (six separate delegations for this suite).
- **Do not** ask a sub-agent to run “all tests” in a single message.

## Setup

Create the results directory if it does not exist: `tests/agent/tmp/`

The results file will be written to: `tests/agent/tmp/test-results.md`

Sub-agents need a working environment: point them at [tests/AGENTS.md](../../AGENTS.md) for cwd and setup check conventions. **You** may run the same setup check from `tests/AGENTS.md` once if the user asked for a full run and you need to confirm the CLI is usable—do **not** use that as an excuse to run individual scenario prompts yourself.

## Delegation (by path only)

For **each** row below, spawn **one** sub-agent. Give the sub-agent **only**:

1. The absolute or repo-relative path to that scenario file (the sub-agent loads and follows that file).
2. A pointer to [tests/AGENTS.md](../../AGENTS.md) for `cd` / `list-models` / provider layout.

**Sub-agent task wording (use verbatim structure):**

> Run **one** agent integration scenario. Open and follow **only** the instructions in: `<PATH>`. Use `tests/AGENTS.md` for where to `cd` and how to pick `--model`. When finished, reply with a single line: `RESULT: PASS` or `RESULT: FAIL` and one short reason.

Collect that `RESULT` line (and optional notes) from each sub-agent. **You** record PASS/FAIL from the sub-agent’s stated result—do not re-derive verdicts by reading scenario files.

### Scenario list (paths only)

| # | Path |
|---|------|
| 1 | `tests/agent/scenarios/test-hello-world.md` |
| 2 | `tests/agent/scenarios/test-large-output.md` |
| 3 | `tests/agent/scenarios/test-tool-calling.md` |
| 4 | `tests/agent/scenarios/test-session-awareness.md` |
| 5 | `tests/agent/scenarios/test-multi-tool.md` |
| 6 | `tests/agent/scenarios/test-error-handling.md` |

Session-specific note (for your planning only, **do not** read the file): scenario 4 may require **two** CLI invocations in the **sub-agent**; still one sub-agent for that file.

## Result format

After all six sub-agents return, write `tests/agent/tmp/test-results.md`:

```markdown
# Agent Integration Test Results

Run at: <timestamp>

| # | Test | Result | Notes |
|---|------|--------|-------|
| 1 | test-hello-world | PASS/FAIL | <from sub-agent> |
| 2 | test-large-output | PASS/FAIL | <from sub-agent> |
| 3 | test-tool-calling | PASS/FAIL | <from sub-agent> |
| 4 | test-session-awareness | PASS/FAIL | <from sub-agent> |
| 5 | test-multi-tool | PASS/FAIL | <from sub-agent> |
| 6 | test-error-handling | PASS/FAIL | <from sub-agent> |

## Summary

Passed: X/6
Failed: Y/6

<Any notable observations>
```

## Final report

Print the results table to the terminal.

Report overall status as either:

- `ALL TESTS PASSED` — if all 6 scenarios passed
- `SOME TESTS FAILED` — if any scenario failed; list which ones

See also: [README.md](./README.md) (scenario template).
