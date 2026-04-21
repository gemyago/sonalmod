# Plan: Expose Tool Call Details via API and Render in UI

## 1. Introduction / Overview

When the agent uses tools (e.g. `workspacefs_write_file`, `workspacefs_list_workspaces`), the underlying ADK framework stores complete tool call data—request parameters (name, ID, arguments) and tool responses—in session events. However, this information is **silently dropped** during event mapping, so it never reaches the SSE stream or the UI.

**Goal:** Expose tool call request and response data through the existing SSE stream API, and render them in the UI as expandable/collapsible blocks. Each block shows the tool name as a brief title when collapsed, and full request arguments + response data when expanded. Collapsed by default to save space.

**Non-goals:**
- Editing or re-running tool calls from the UI
- Streaming partial tool call arguments (incremental arg building)
- Real-time progress indicators for tool execution
- Changing the tool execution flow itself

---

## 2. Business Logic

- **Tool call events** are part of the agent's turn. A single turn may contain: text → tool call(s) → tool response(s) → more text.
- The API should expose tool call data as **part types** within the existing `agent` SSE event structure. An `agent` event's `content.parts` array can now carry text parts, tool call parts, or tool result parts (mirroring the underlying `genai.Content.Parts`).
- The UI groups tool calls with the assistant turn they belong to. Each assistant message can have zero or more associated tool calls.
- Tool calls appear in both **live streaming** (events from an active run) and **history replay** (events from `readSession`).
- During **streaming**, partial chunks continue to carry text only (tool parts are stripped from partials by the existing `dropToolPartsFromPartialChunk`). Tool call data appears only in non-partial events.

---

## 3. High-Level Architecture

| Component | Role | Module |
|-----------|------|--------|
| `SessionEvent` / `SessionEventPart` | Internal projection of ADK events; currently text-only, needs tool call support | `runtime/internal` |
| OpenAPI spec (`openapi.yaml`) | Defines the SSE event schema; `AgentStreamPart` needs tool call/result fields | `runtime/internal/agentapi` |
| `AgentAPIStreamEventMapper` | Maps `SessionEvent` → `StreamEvent`; needs to map tool parts | `runtime/internal/agentapi` |
| Generated Go types (`api.gen.go`) | Auto-generated from OpenAPI; regenerated after spec changes | `runtime/internal/agentapi` |
| Generated TS types (`agentapi.generated.ts`) | Auto-generated from OpenAPI; regenerated after spec changes | `apps/sonal-ui` |
| `streamState.ts` | Stream reducer; needs to track tool calls in state | `apps/sonal-ui` |
| `ToolCallBlock.svelte` | New component for expandable/collapsible tool call rendering | `apps/sonal-ui` |
| `Chat.svelte` | Chat page; needs to render tool calls within assistant turns | `apps/sonal-ui` |

---

## 4. Detailed Architecture

### 4.1 Extend `SessionEventPart` with Tool Call Data (`runtime/internal/session_event.go`)

Currently, `SessionEventPart` only holds `Text string`. Add two new struct types and optional pointer fields:

```go
type SessionEventFunctionCall struct {
    ID   string
    Name string
    Args map[string]any
}

type SessionEventFunctionResponse struct {
    ID       string
    Name     string
    Response map[string]any
}

type SessionEventPart struct {
    Text             string
    FunctionCall     *SessionEventFunctionCall
    FunctionResponse *SessionEventFunctionResponse
}
```

Update `sessionEventContentFromGenAI` to map all non-nil `genai.Part` entries:
- `p.Text != ""` → `SessionEventPart{Text: p.Text}` (as today)
- `p.FunctionCall != nil` → `SessionEventPart{FunctionCall: &SessionEventFunctionCall{...}}`
- `p.FunctionResponse != nil` → `SessionEventPart{FunctionResponse: &SessionEventFunctionResponse{...}}`

This ensures tool call data flows through `MapADKSessionEvent` → `EventBus` → mapper → SSE.

### 4.2 Extend OpenAPI Spec (`runtime/internal/agentapi/openapi.yaml`)

Add two new schemas:

```yaml
ToolCallData:
  type: object
  description: Tool invocation requested by the model.
  required: [id, name]
  properties:
    id:
      type: string
      description: Unique call ID matching the tool result.
    name:
      type: string
      description: Tool function name.
    args:
      type: object
      additionalProperties: true
      description: Arguments passed to the tool (JSON object).
  additionalProperties: false

ToolResultData:
  type: object
  description: Result returned by a tool invocation.
  required: [id, name]
  properties:
    id:
      type: string
      description: Call ID matching the original tool call.
    name:
      type: string
      description: Tool function name.
    response:
      type: object
      additionalProperties: true
      description: Tool output (JSON object).
  additionalProperties: false
```

Modify `AgentStreamPart`:
- Remove `text` from `required` (it becomes optional).
- Remove `additionalProperties: false` (or keep it and add the new fields).
- Add optional `toolCall` (`$ref: ToolCallData`) and `toolResult` (`$ref: ToolResultData`).
- Update description to note that exactly one of `text`, `toolCall`, or `toolResult` is present per part.

```yaml
AgentStreamPart:
  type: object
  description: |
    A segment in streamed agent output. Exactly one of text, toolCall, or toolResult is present.
  properties:
    text:
      type: string
    toolCall:
      $ref: '#/components/schemas/ToolCallData'
    toolResult:
      $ref: '#/components/schemas/ToolResultData'
  additionalProperties: false
```

After editing the spec, regenerate Go types:
```sh
go generate ./internal/agentapi
```

### 4.3 Update Stream Event Mapper (`runtime/internal/agentapi/stream_event_mapper.go`)

`sessionEventContentToAgentStream` currently only maps parts with non-empty `Text`. Update to also map:
- `SessionEventPart.FunctionCall` → `AgentStreamPart{ToolCall: &ToolCallData{...}}`
- `SessionEventPart.FunctionResponse` → `AgentStreamPart{ToolResult: &ToolResultData{...}}`

The mapper should skip parts where all fields are zero (no text, no function call, no function response).

### 4.4 Regenerate TypeScript Types (`apps/sonal-ui`)

After the OpenAPI spec change, regenerate:
```sh
cd apps/sonal-ui && make generate-api
```

New re-exports in `types.ts`:
```typescript
export type ToolCallData = components['schemas']['ToolCallData']
export type ToolResultData = components['schemas']['ToolResultData']
```

### 4.5 Extend Stream State (`apps/sonal-ui/src/lib/agentapi/streamState.ts`)

Add a `ToolCallEntry` type and track tool calls in the stream state:

```typescript
export interface ToolCallEntry {
  id: string
  name: string
  args: Record<string, unknown>
  response?: Record<string, unknown>
}
```

Add `toolCalls: ToolCallEntry[]` to `AgentRunStreamState`.

Update `applyAgent`:
- When processing `content.parts`, separate text parts from tool call/result parts.
- Text parts → same accumulation logic as today.
- `toolCall` parts → append new `ToolCallEntry` to `state.toolCalls`.
- `toolResult` parts → find matching entry by `id` in `state.toolCalls` and set `response`.

Extend `ChatTranscriptMessage`:
```typescript
export type ChatTranscriptMessage = {
  role: 'user' | 'assistant'
  text: string
  toolCalls?: ToolCallEntry[]
}
```

Update `flushAssistantMessage`: include accumulated tool calls in the flushed row when present (even if text is empty, if there are tool calls, flush them).

Update `applyIdleHistoryAgentEvent`: same tool call tracking and flushing logic for history replay.

### 4.6 New Component: `ToolCallBlock.svelte` (`apps/sonal-ui/src/components/ToolCallBlock.svelte`)

A collapsible/expandable block using the HTML `<details>/<summary>` pattern:
- **Collapsed (default):** Shows a brief summary line: tool name (e.g., "workspacefs_write_file").
- **Expanded:** Shows the full request arguments (as formatted JSON) and, if available, the response data (also as formatted JSON).
- The block should have a distinct visual style (e.g., a border, slightly different background) to distinguish it from text messages.

Props:
```typescript
interface Props {
  name: string
  args: Record<string, unknown>
  response?: Record<string, unknown>
}
```

### 4.7 Update `Chat.svelte` (`apps/sonal-ui/src/pages/Chat.svelte`)

**Committed messages:** When rendering each message in the transcript, if `m.toolCalls?.length`, render `ToolCallBlock` components after the text bubble (or instead of the text bubble if text is empty).

**Streaming state:** In the turn-activity area, after the streaming text (or "Thinking…"), render any `streamState.toolCalls` entries as `ToolCallBlock` components.

**On `done`:** Commit `assistantTurnText` + `toolCalls` to the messages array as a single transcript row, then reset both fields.

### 4.8 Update UI Wireframe (`apps/sonal-ui/ui-wireframe.md`)

Add a new section or update the Chat section to describe tool call block behavior:
- Appearance within assistant turns
- Collapsed/expanded states
- Data shown in each state

---

## 5. Key Architectural Decisions

1. **Tool call data as part types within `agent` events** (not new top-level SSE event types). This mirrors the underlying `genai.Content.Parts` model where text, function calls, and function responses coexist as parts. It avoids adding new discriminator values to the `StreamEvent` union and keeps the change smaller.

2. **`AgentStreamPart` becomes polymorphic** (optional `text`, `toolCall`, or `toolResult` — exactly one per part). This is the simplest schema evolution. A discriminated union (`oneOf`) was considered but adds complexity for v0.1.0.

3. **Tool calls are grouped with assistant messages** (`ChatTranscriptMessage.toolCalls`), not as separate transcript items. This keeps the transcript model simple and naturally groups tool activity with the assistant turn it belongs to.

4. **HTML `<details>/<summary>` for collapse/expand.** Native browser pattern, accessible by default, no JavaScript state needed for the toggle.

5. **Partial streaming chunks continue to strip tool parts** (`dropToolPartsFromPartialChunk` stays unchanged). Tool call data only appears in non-partial events. This avoids showing incomplete tool call information during streaming.

6. **Text and tool calls within a turn are shown sequentially** (all text first, then tool calls). Precise interleaving order within a single turn is not preserved in the MVP. This simplifies state management significantly.

---

## 6. Uncertainties

- **Multiple tool calls in one event:** ADK can emit multiple `FunctionCall` parts in a single event (parallel tool calls). The plan handles this (each becomes a separate `ToolCallEntry`), but the UI layout for many simultaneous tool calls should be verified visually.
- **Tool call ID matching:** The plan assumes `FunctionCall.ID` and `FunctionResponse.ID` always match. If ADK ever emits responses without IDs, the matcher would need a fallback (e.g., match by name + order).
- **Large arguments/responses:** Some tool calls may have very large `args` or `response` JSON (e.g., file content). The collapsed default mitigates this, but the expanded view might need truncation or scrolling. Not addressed in MVP.
- **Error during tool execution:** If a tool fails, the `FunctionResponse.Response` may contain an error object. The UI should render this the same way (just show the response JSON). No special error styling in MVP.

---

## 7. Related Files

### Files to modify

| File | Change |
|------|--------|
| `runtime/internal/session_event.go` | Add `SessionEventFunctionCall`, `SessionEventFunctionResponse` structs; update `SessionEventPart`; update `sessionEventContentFromGenAI` |
| `runtime/internal/agentapi/openapi.yaml` | Add `ToolCallData`, `ToolResultData` schemas; modify `AgentStreamPart` |
| `runtime/internal/agentapi/api.gen.go` | Regenerated (do not edit manually) |
| `runtime/internal/agentapi/stream_event_mapper.go` | Map tool call/result parts to API types |
| `runtime/internal/agentapi/stream_event_mapper_test.go` | Add tests for tool call/result mapping |
| `apps/sonal-ui/src/lib/agentapi/agentapi.generated.ts` | Regenerated (do not edit manually) |
| `apps/sonal-ui/src/lib/agentapi/types.ts` | Add `ToolCallData`, `ToolResultData` re-exports |
| `apps/sonal-ui/src/lib/agentapi/streamState.ts` | Add `ToolCallEntry`, extend state and reducer |
| `apps/sonal-ui/src/lib/agentapi/streamState.test.ts` | Add tests for tool call tracking |
| `apps/sonal-ui/src/pages/Chat.svelte` | Render tool calls in transcript and streaming area |
| `apps/sonal-ui/src/pages/Chat.test.ts` | Add/update tests for tool call rendering |
| `apps/sonal-ui/ui-wireframe.md` | Document tool call block behavior |

### Files to create

| File | Purpose |
|------|---------|
| `apps/sonal-ui/src/components/ToolCallBlock.svelte` | Expandable/collapsible tool call block component |

### Files NOT modified (unchanged behavior)

| File | Reason |
|------|--------|
| `runtime/internal/genkit_adapter.go` | `dropToolPartsFromPartialChunk` stays as-is; tool parts in non-partial events flow through unchanged |
| `runtime/internal/agentapi/sse.go` | SSE writer is generic over `StreamEvent`; no changes needed |
| `runtime/internal/background_runner.go` | Passes `SessionEvent` objects through; transparent to content |

---

## 8. Task List

All tasks follow TDD. Each task must leave the codebase in a buildable/passing state per the module-specific completion protocol.

---

**Task 1.1: Extend `SessionEventPart` with FunctionCall/FunctionResponse support**
- In `runtime/internal/session_event.go`:
  - Add `SessionEventFunctionCall` struct (fields: `ID`, `Name`, `Args map[string]any`)
  - Add `SessionEventFunctionResponse` struct (fields: `ID`, `Name`, `Response map[string]any`)
  - Add optional pointer fields `FunctionCall *SessionEventFunctionCall` and `FunctionResponse *SessionEventFunctionResponse` to `SessionEventPart`
- Write failing tests in `runtime/internal/session_event_test.go` (new file):
  - `sessionEventContentFromGenAI` maps `genai.Part` with `FunctionCall` → `SessionEventPart.FunctionCall`
  - `sessionEventContentFromGenAI` maps `genai.Part` with `FunctionResponse` → `SessionEventPart.FunctionResponse`
  - Mixed content (text + FunctionCall + FunctionResponse) maps all parts
  - Nil/empty FunctionCall/FunctionResponse parts are skipped
- Run affected tests: `go test -v ./internal/ --run TestSessionEvent`
  - Verify failure is expectation-based (not compilation errors)
- Implement the mapping in `sessionEventContentFromGenAI`:
  - For each `genai.Part`: check `FunctionCall`, `FunctionResponse` in addition to `Text`
  - Map fields: `FunctionCall.ID` → `SessionEventFunctionCall.ID`, `.Name` → `.Name`, `.Args` → `.Args`
  - Map fields: `FunctionResponse.ID` → `.ID`, `.Name` → `.Name`, `.Response` → `.Response`
- Run affected tests: `go test -v ./internal/ --run TestSessionEvent`
  - Verify all tests pass
- Run: `make lint` and `make test` from `runtime/`
- Write summary to `doc/implementation/tool-call-visibility/summary-task-1.1.md`
- All checks from completion protocol must be passed

---

**Task 2.1: Update OpenAPI spec and regenerate Go types**
- In `runtime/internal/agentapi/openapi.yaml`:
  - Add `ToolCallData` schema (required: `id`, `name`; optional: `args`)
  - Add `ToolResultData` schema (required: `id`, `name`; optional: `response`)
  - Modify `AgentStreamPart`: remove `text` from `required`, add optional `toolCall` and `toolResult` properties, update description
- Regenerate Go code: `go generate ./internal/agentapi`
- Verify the generated `api.gen.go` has the new types (`ToolCallData`, `ToolResultData`) and the updated `AgentStreamPart`
- Run: `make lint` and `make test` from `runtime/`
  - Existing tests should still pass (text-only parts still work; new fields are optional)
- No separate test file needed for schema-only changes
- Write summary to `doc/implementation/tool-call-visibility/summary-task-2.1.md`
- All checks from completion protocol must be passed

---

**Task 2.2: Update stream event mapper to map tool call/result parts**
- In `runtime/internal/agentapi/stream_event_mapper.go`:
  - Update `sessionEventContentToAgentStream` to handle `FunctionCall` and `FunctionResponse` parts
- Write failing tests in `runtime/internal/agentapi/stream_event_mapper_test.go`:
  - Agent event with FunctionCall part → `AgentStreamPart` has `ToolCall` set with correct fields
  - Agent event with FunctionResponse part → `AgentStreamPart` has `ToolResult` set with correct fields
  - Agent event with mixed text + FunctionCall parts → both text and tool call parts in output
  - Agent event with multiple FunctionCall parts (parallel calls) → all mapped
- Run affected tests: `go test -v ./internal/agentapi/ --run TestAgentAPIStreamEventMapper`
  - Verify failure is expectation-based
- Implement the mapping in `sessionEventContentToAgentStream`:
  - For each `SessionEventPart`: check `FunctionCall`, `FunctionResponse` alongside `Text`
  - Map `FunctionCall` → `AgentStreamPart{ToolCall: &ToolCallData{Id: ..., Name: ..., Args: ...}}`
  - Map `FunctionResponse` → `AgentStreamPart{ToolResult: &ToolResultData{Id: ..., Name: ..., Response: ...}}`
- Run affected tests: `go test -v ./internal/agentapi/ --run TestAgentAPIStreamEventMapper`
  - Verify all tests pass
- Run: `make lint` and `make test` from `runtime/`
- Write summary to `doc/implementation/tool-call-visibility/summary-task-2.2.md`
- All checks from completion protocol must be passed

---

**Task 3.1: Regenerate TypeScript types and add re-exports**
- From `apps/sonal-ui`, regenerate: `make generate-api`
- Verify `src/lib/agentapi/agentapi.generated.ts` has `ToolCallData`, `ToolResultData`, and updated `AgentStreamPart` (text optional)
- In `src/lib/agentapi/types.ts`:
  - Add re-exports: `ToolCallData`, `ToolResultData`
- Run: `make lint` and `make test` from `apps/sonal-ui`
  - Existing tests should pass (text-only parts still work)
- Write summary to `doc/implementation/tool-call-visibility/summary-task-3.1.md`
- All checks from completion protocol must be passed

---

**Task 3.2: Extend stream state to track tool calls**
- In `apps/sonal-ui/src/lib/agentapi/streamState.ts`:
  - Add `ToolCallEntry` interface: `{ id: string; name: string; args: Record<string, unknown>; response?: Record<string, unknown> }`
  - Add `toolCalls: ToolCallEntry[]` field to `AgentRunStreamState`
  - Update `createInitialAgentRunStreamState` to initialize `toolCalls: []`
  - Extend `ChatTranscriptMessage` to include optional `toolCalls?: ToolCallEntry[]`
  - Update `snapshotTextFromAgentEvent`: only join parts that have `text` (skip parts with `toolCall` or `toolResult`)
  - Add helper to extract tool call/result parts from an `AgentStreamEvent`
  - Update `applyAgent`: also process `toolCall` and `toolResult` parts, updating `state.toolCalls`
  - Update `flushAssistantMessage`: include `toolCalls` in the flushed row; flush when there are tool calls even if text is empty
  - Update `applyIdleHistoryAgentEvent`: handle tool call tracking and flushing for history replay
- Write failing tests in `src/lib/agentapi/streamState.test.ts`:
  - Agent event with `toolCall` part adds entry to `state.toolCalls`
  - Agent event with `toolResult` part updates matching entry's `response`
  - Mixed text + toolCall parts: text is accumulated, tool call is tracked
  - `applyIdleHistoryAgentEvent` flushes tool calls with assistant message on `turnComplete`
  - Agent event with only tool call (no text) still creates flush-able state
- Run affected tests: `npm run test:run` from `apps/sonal-ui`
  - Verify failure is expectation-based
- Implement the logic
- Run tests again; verify all pass
- Run: `make lint` and `make test` from `apps/sonal-ui`
- Write summary to `doc/implementation/tool-call-visibility/summary-task-3.2.md`
- All checks from completion protocol must be passed

---

**Task 4.1: Create `ToolCallBlock` component**
- Create `apps/sonal-ui/src/components/ToolCallBlock.svelte`
- Props: `name: string`, `args: Record<string, unknown>`, `response?: Record<string, unknown>`
- Structure:
  - `<details>` element (collapsed by default)
  - `<summary>`: tool name (e.g., "workspacefs_write_file") with a visual indicator (e.g., a small icon or label like "Tool call")
  - Inside details: formatted JSON of `args` labeled "Arguments", and if `response` is present, formatted JSON of `response` labeled "Response"
  - Use `JSON.stringify(data, null, 2)` within `<pre>` for formatting
- Styling:
  - Distinct visual appearance from text bubbles (e.g., subtle border, muted background)
  - `<summary>` should have cursor pointer and clear expand/collapse affordance
  - Fits within the chat layout (max-width matches assistant bubbles)
- No complex logic → tests are optional for this component (visual only)
- Run: `make lint` from `apps/sonal-ui`
- Write summary to `doc/implementation/tool-call-visibility/summary-task-4.1.md`
- All checks from completion protocol must be passed

---

**Task 4.2: Render tool calls in Chat.svelte**
- Import `ToolCallBlock` in `Chat.svelte`
- **Committed messages section:** For each message in the transcript loop:
  - If `m.toolCalls?.length`, render `ToolCallBlock` for each tool call below the text bubble (or standalone if text is empty)
- **Streaming section (turn-activity):** Below the streaming text / "Thinking…" indicator:
  - If `streamState.toolCalls.length > 0`, render `ToolCallBlock` for each entry
- **On `done` handler:** Commit both `assistantTurnText` and `toolCalls` to the messages array:
  - `messages = [...messages, { role: 'assistant', text: assistantTurnText, toolCalls: streamState.toolCalls.length ? [...streamState.toolCalls] : undefined }]`
  - Reset `streamState` including clearing `toolCalls`
- Same logic for the `loadSession` done handler
- Write or update tests in `Chat.test.ts` for tool call rendering (if existing test patterns support it)
- Run: `make lint` and `make test` from `apps/sonal-ui`
- Write summary to `doc/implementation/tool-call-visibility/summary-task-4.2.md`
- All checks from completion protocol must be passed

---

**Task 4.3: Update UI wireframe**
- Update `apps/sonal-ui/ui-wireframe.md`:
  - Add tool call block description to the Chat section
  - Document: appearance within assistant turns, collapsed/expanded states, data shown in each state
  - Note: blocks are collapsed by default, show tool name in summary, expand to show args + response JSON
- Write summary to `doc/implementation/tool-call-visibility/summary-task-4.3.md`
- All checks from completion protocol must be passed

---

**Task 5.1: Compress implementation summaries**
- Follow [compress-implementation-summaries.md](/.context/compress-implementation-summaries.md) to compress the implementation summaries.
