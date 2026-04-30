# OpenCode ACP Capability Map

This map is based on observed ACP probe runs from 2026-04-22, not assumptions.

| Capability | Status | Evidence | Notes for Sonalmod |
|---|---|---|---|
| Capability negotiation (`initialize`) | validated | Probe run: request `id=1`, response `id=1` | `protocolVersion` is mandatory in params. |
| Session creation (`session/new`) | validated | Probe run: request `id=2`, response `id=2` | `cwd` and `mcpServers` are required by OpenCode. |
| Prompt streaming (`session/prompt` + `session/update`) | validated | Probe run: request `id=3`, `session/update` stream, final `id=3` result | Prompt payload must be an array of text blocks. |
| Cancellation (`session/cancel`) | not advertised | Probe run: request `id=4`, error `code=-32601` | OpenCode returned `"Method not found": session/cancel`. Defer user-facing cancel for first slice. |
| Session resume (`session/load`) | advertised but untested | `agentCapabilities.loadSession=true` observed in probe | Capability is advertised; run a dedicated `--load-session` probe before relying on it. |
| Session listing (`session/list`) | advertised but untested | `sessionCapabilities.list` observed in probe | Treat as optional until explicitly exercised. |
| Session close (`session/close`) | not advertised | No `session/close` capability in initialize result | Keep out of first OpenCode integration. |
| MCP server injection | deferred | `session/new` accepted with `mcpServers: []` in transcript | Non-empty server injection behavior was not exercised in this phase. |
| Slash-command limitations | deferred | `session/update.availableCommands` was observed; `/undo` and `/redo` not explicitly probed | Use OpenCode docs + dedicated probes before exposing slash command guarantees. |
| Permission/tool-related signals | validated | `session/update` included command inventory and usage updates | The runtime emits actionable metadata; wiring can expose these as diagnostics. |

## Validated ACP subset for next phase

Sonalmod Phase 2 and Phase 3 should assume only:

1. `initialize` with explicit `protocolVersion`
2. `session/new` with explicit `cwd` and `mcpServers`
3. `session/prompt` with prompt block arrays
4. `session/update` streaming and final prompt response

Everything else remains optional until verified.
