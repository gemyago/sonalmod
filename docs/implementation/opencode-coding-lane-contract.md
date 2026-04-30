# OpenCode Coding Lane Contract (Superseded)

This document is retained as historical context for the earlier OpenCode-specific
runtime lane. The active public contract was replaced by profile execution
settings in `replace-opencode-with-profile-execution-settings`.

## Current Runtime Contract

- Standard runtime execution uses `POST /agent-runs` and
  `POST /sessions/{sessionId}/agent-runs` with a required `profileName`.
- Agent profiles now own execution mode through `executionSettings`.
- Omitted or explicit `regular` mode uses the built-in runner and
  `executionSettings.defaultModel`.
- `acp-stdio` mode stores process launch settings on the profile itself:
  command, args, and optional `cwd`.
- Public `/opencode-bindings`, `/opencode-bindings/{bindingName}`, and
  `/opencode-launches` endpoints were removed.

## OpenCode-Specific Scope That Remains

OpenCode may still be used as the executable for an `acp-stdio` profile, and the
validated ACP subset remains documented in
[opencode-acp-capability-map.md](./opencode-acp-capability-map.md). Those notes
describe executable behavior, not public runtime endpoints.
