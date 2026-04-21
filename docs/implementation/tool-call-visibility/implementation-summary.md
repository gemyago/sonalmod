# Implementation Summary: Expose Tool Call Details via API and Render in UI

**Plan:** [plan-tool-call-visibility.md](./plan-tool-call-visibility.md)

## Overview

Runtime session events now carry `FunctionCall` / `FunctionResponse` parts from ADK through `SessionEventPart`, the agent API OpenAPI spec and Go/TS generators expose `toolCall` / `toolResult` on stream parts, and the mapper sends those parts over SSE. The Svelte app tracks tool calls in stream state, shows them in the chat transcript and during streaming via `ToolCallBlock`, and the UI wireframe documents the behavior.

## Tasks

### Task 1.1: Extend `SessionEventPart` with FunctionCall/FunctionResponse support

Added `SessionEventFunctionCall` and `SessionEventFunctionResponse`, extended `SessionEventPart` with optional pointers, and updated `sessionEventContentFromGenAI` to map text, function calls, and responses; tests cover nil/empty/mixed and multiple parallel parts.

### Task 2.1: Update OpenAPI spec and regenerate Go types

The OpenAPI spec gained `ToolCallData` and `ToolResultData`, and `AgentStreamPart` was updated for optional tool fields and nullable text; types were regenerated and call sites were fixed for `Text` as `*string` and for shorter generated enum constant names.

### Task 2.2: Update stream event mapper to map tool call/result parts

Extracted `sessionEventPartToAgentStream` so each session part maps to agent stream parts (text, tool call, tool result); args/response maps are copied before use, zero parts are skipped, and tests cover function call/response, mixed content, and parallel calls.

### Task 3.1: Regenerate TypeScript types and add re-exports

Regenerated `agentapi.generated.ts` via `make generate-api` so it includes `ToolCallData`, `ToolResultData`, and the updated `AgentStreamPart`; added re-exports for the new schema types in `types.ts`.

### Task 3.2: Extend stream state to track tool calls

Stream state tracks tool invocations with `ToolCallEntry` / `toolCalls`, applies tool parts separately from text, and flushes assistant rows that include tool calls (including tool-only turns); history replay resets tool call state as specified.

### Task 4.1: Create `ToolCallBlock` component

Added `ToolCallBlock.svelte` with props for tool name, arguments, and optional response; it uses a collapsible `<details>/<summary>` (collapsed by default), shows a badge and monospace tool name, and renders formatted JSON for arguments and response using shared theme CSS variables.

### Task 4.2: Render tool calls in Chat.svelte

`Chat.svelte` renders `ToolCallBlock` for each tool call on committed messages and for live `streamState.toolCalls` during a turn; submit and `loadSession` attach tool calls when committing the assistant row. A fixture and `Chat.test.ts` cover committed tool-call UI.

### Task 4.3: Update UI wireframe

The Chat section of `apps/sonal-ui/ui-wireframe.md` documents tool-call blocks: placement in assistant turns, collapsed summary vs expanded JSON for arguments/response, behavior in live streaming and committed transcript, and styling.

## Deviations & notes

- **Task 2.1:** `go generate ./internal/agentapi` failed under Go 1.26 because the oapi-codegen dependency did not compile; regeneration used a pre-built `oapi-codegen` binary. Regeneration shortened several generated enum identifiers, so non-generated code was updated to match.
- **Task 4.2:** Tool calls in the streaming area are rendered outside `{#if streamState.busy}` so they can stay visible briefly after busy ends and before `done`; in practice this is hard to observe because `done` resets state immediately.

## Completion

- Lint: ✓
- Type check: ✓
- Tests: ✓
