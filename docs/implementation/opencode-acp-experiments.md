# OpenCode ACP Experiments

## Goal

Capture observed ACP behavior from `opencode acp` and record the validated ACP subset Sonalmod should rely on for the first integration slice.

## Environment assumptions

- `opencode` binary is installed and available on `PATH`.
- Local OpenCode auth is configured (`opencode auth login` already completed).
- Commands run from `tests/agent/integration-cli`.

## Commands run

```bash
go run . acp \
  --agent-command opencode \
  --agent-arg acp \
  --cwd /Users/jenya/projects/sonalmod \
  --prompt "hello from sonalmod ACP probe" \
  --transcript ../../docs/implementation/opencode-acp-transcripts/initialize-and-prompt.jsonl
```

```bash
go run . acp \
  --agent-command opencode \
  --agent-arg acp \
  --cwd /Users/jenya/projects/sonalmod \
  --prompt "write a long response that I will cancel after one second" \
  --cancel-after 1s \
  --transcript ../../docs/implementation/opencode-acp-transcripts/cancel.jsonl
```

## Observed behavior

- `initialize` requires `protocolVersion` in params.
- `session/new` requires both `cwd` and `mcpServers` params.
- `session/prompt` requires prompt content as an array of content blocks, not a plain string.
- OpenCode advertised:
  - `agentCapabilities.loadSession: true`
  - `agentCapabilities.sessionCapabilities.list`
  - `agentCapabilities.sessionCapabilities.resume`
  - `agentCapabilities.sessionCapabilities.fork`
  - `agentCapabilities.mcpCapabilities.http` and `.sse`
- Prompt streaming is emitted via `session/update` notifications (`agent_thought_chunk`, `agent_message_chunk`, `usage_update` observed).
- `session/cancel` returned `"Method not found"` in this probe and is not validated for the first integration.

## Session lifecycle findings

- Validated lifecycle path: `initialize` -> `session/new` -> `session/prompt` -> streamed `session/update` -> prompt completion response.
- `session/load` was advertised but not executed in this probe run.
- `session/list` was advertised but not executed in this probe run.
- `session/close` was not advertised in the initialize capability payload.

## Run outputs

- Probe output files are intentionally local-only and are not committed.
- Re-run the two commands above whenever protocol verification is needed.
- Keep the committed capability map as the durable summary of observed behavior.

## Data handling note

Raw ACP outputs may contain sensitive local/runtime metadata (absolute paths, session identifiers, command inventories, and model/runtime payloads). Keep generated transcript files out of git.
