# High level tests (integration/e2e)

This module contains high level tests for the project, like e2e or integration tests.

## Agent tests

Use this instruction when user requests to run agent e2e tests. User must do some manual setup which AI should not be concerned with. AI should assume that everything is configured and ready to run.

The user will typically say something like "run agent integration tests" or "run agent e2e tests".

### Setup check

AI must run commands below to check if the setup is correct. If anything fails, AI must not attempt to fix anything or do any investigation if it's not requested by user. It should report the error to the user.

Always run commands from module root.

```bash
# CD to module root
cd tests/agent/integration-cli

# List configured models. Lines look like: * provider/model
go run . list-models

# Use one model name from the list (provider/model, without the leading "* ") for the check.
go run . run \
  --model '<provider/model>' \
  --prompt "hello"
```

Replace `'<provider/model>'` with a real name printed by `list-models`.

## integration-cli ACP probe mode

`tests/agent/integration-cli` also includes an `acp` subcommand for manual protocol probing against an ACP-capable agent.

Prerequisites:
- `opencode` is installed and available in `PATH`
- the local `opencode acp` environment is authenticated and ready for interactive use

Example command shape:

```bash
cd tests/agent/integration-cli
go run . acp \
  --agent-command opencode \
  --agent-arg acp \
  --prompt "hello from integration-cli" \
  --transcript ../../docs/implementation/opencode-acp-transcripts/probe.jsonl
```

Optional flags:
- `--cwd` to set the ACP agent working directory
- `--load-session` to call `session/load` when advertised by agent capabilities
- `--cancel-after` to issue `session/cancel` after a delay

### Run agent test scenarios via the master orchestrator

Once setup check succeeds, follow [./agent/scenarios/master.md](./agent/scenarios/master.md)

## Task Completion Protocol

Repository level task completion protocol **MUST ALWAYS** be followed. If you didn't follow it, this means task is not complete.
