# Phase 1: ACP Discovery And Capability Map - Research

**Created:** 2026-04-22
**Scope:** Official ACP and OpenCode sources relevant to the first OpenCode-backed integration

## Research Question

What do we need to know to plan an ACP discovery phase well for OpenCode?

## Primary Sources

- OpenCode ACP docs: `https://opencode.ai/docs/acp/`
- ACP overview: `https://agentclientprotocol.com/protocol/overview`
- ACP session setup: `https://agentclientprotocol.com/protocol/session-setup`
- ACP schema reference: `https://agentclientprotocol.com/protocol/schema`
- ACP `session/close` RFD: `https://agentclientprotocol.com/rfds/session-close`

## Confirmed Facts

### OpenCode behavior

- OpenCode exposes ACP through the `opencode acp` command.
- OpenCode runs as an ACP-compatible subprocess and communicates over JSON-RPC via stdio.
- OpenCode documents that ACP mode supports the same core functionality as terminal mode, including built-in tools, custom tools and slash commands, MCP servers from OpenCode config, project `AGENTS.md` rules, and permissions.
- OpenCode also notes that some built-in slash commands, specifically `/undo` and `/redo`, are currently unsupported through ACP.

### ACP baseline lifecycle

- ACP uses JSON-RPC 2.0 with request/response methods plus one-way notifications.
- The normal flow is `initialize`, optional `authenticate`, `session/new` or `session/load`, then `session/prompt`, with `session/update` notifications during execution.
- Clients may send `session/cancel` during an active prompt turn to interrupt processing.
- Sessions are scoped by an absolute `cwd`, which the agent must use for that session.

### Capability gating

- `session/load` is only valid when the agent advertises `loadSession`.
- `session/list` is only valid when the agent advertises `sessionCapabilities.list`.
- MCP over stdio is mandatory in ACP agents; HTTP and SSE MCP transports are optional and capability-gated.
- `session/close` is still in preview as an RFD-backed capability and must not be assumed unless advertised.

## Planning Implications

### What Sonalmod should validate in Phase 1

- The exact `initialize` response OpenCode returns, including capability flags Sonalmod can rely on.
- The smallest end-to-end session lifecycle Sonalmod needs for an OpenCode-backed coding agent.
- How OpenCode emits progress and completion through `session/update`.
- Whether cancellation works well enough for Sonalmod to expose it in later phases.
- Whether session resume is available and usable for Sonalmod's future session model.
- Which optional ACP features should remain out of scope because OpenCode does not advertise or reliably implement them.

### Repo-level design consequences

- Sonalmod needs a probe client that preserves raw ACP envelopes, not just summarized output.
- The probe should treat capabilities as runtime data, not compile-time assumptions.
- The future integration boundary should separate general agent configuration from ACP session wiring only after Phase 1 proves where that line actually holds.

## Risks And Unknowns

- OpenCode documentation confirms ACP support, but not every optional ACP capability needed by Sonalmod's eventual design.
- `session/close` is not part of the stable baseline and may be absent even if useful.
- OpenCode's note about unsupported slash commands is a reminder that "same as terminal" still has exceptions.
- The repo currently has no ACP-specific path in `tests/agent/integration-cli`, so the first implementation needs to add one before deeper conclusions are possible.

## Recommended Planning Direction

1. Extend `tests/agent/integration-cli` with an ACP mode in Go, reusing its existing CLI and testing patterns.
2. Make the probe record transcripts for `initialize`, `session/new`, `session/prompt`, `session/cancel`, and `session/load` when supported.
3. Run the probe against OpenCode with a minimal set of scripted scenarios.
4. Publish an OpenCode capability map that distinguishes:
   - Supported and validated
   - Supported but not yet relied upon
   - Unsupported
   - Unclear / deferred
5. Feed those findings back into `.planning` docs before starting the real integration phase.

## Validation Architecture

- Automated checks should focus on the probe client itself: JSON-RPC framing, capability parsing, transcript recording, and command-line behavior.
- Real OpenCode experiments are partially manual or environment-dependent and should be captured as reproducible commands plus stored transcripts.
- The phase should finish with both automated probe tests and a human-reviewable capability map generated from actual experiment runs.

---

*Research based on official OpenCode and ACP documentation reviewed on 2026-04-22.*
