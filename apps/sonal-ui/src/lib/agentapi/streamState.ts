import type {
  AgentStreamEvent,
  SessionBoundEvent,
  SessionStatusEvent,
  StreamErrorEvent,
  StreamEvent,
} from './types'

export interface ToolCallEntry {
  id: string
  name: string
  args: Record<string, unknown>
  response?: Record<string, unknown>
}

/**
 * UI-facing state for a single in-flight agent run (one POST → SSE until done/error).
 * `applyAgentStreamEvent` only interprets stream events; the caller sets `busy` when
 * starting a run and may reset fields when starting a new turn.
 */
export interface AgentRunStreamState {
  sessionId: string | null
  /** False after `done` or `error`; true while chunks may still arrive. */
  busy: boolean
  /**
   * Assistant text for the current turn. Each model `agent` event appends text from
   * `content.parts[].text`. Events with `content.role === 'user'` are ignored (ADK emits user and
   * model turns as `agent` SSE events).
   */
  assistantTurnText: string
  error: string | null
  /** Set by `sessionStatus` event: true when a run is active, false when idle. */
  sessionActive: boolean | null
  /** Tool calls accumulated in the current assistant turn. */
  toolCalls: ToolCallEntry[]
}

export function createInitialAgentRunStreamState(
  overrides?: Partial<AgentRunStreamState>,
): AgentRunStreamState {
  return {
    sessionId: null,
    busy: true,
    assistantTurnText: '',
    error: null,
    sessionActive: null,
    toolCalls: [],
    ...overrides,
  }
}

function snapshotTextFromAgentEvent(ev: AgentStreamEvent): string | undefined {
  if (ev.content?.role === 'user') {
    return undefined
  }
  const parts = ev.content?.parts
  if (!parts?.length) {
    return undefined
  }
  // Only join text parts; skip toolCall/toolResult parts
  return parts
    .filter((p) => p.text !== undefined && p.toolCall === undefined && p.toolResult === undefined)
    .map((p) => p.text)
    .join('')
}

function applySessionBound(state: AgentRunStreamState, event: SessionBoundEvent): AgentRunStreamState {
  return { ...state, sessionId: event.sessionId }
}

function applySessionStatus(state: AgentRunStreamState, event: SessionStatusEvent): AgentRunStreamState {
  return { ...state, sessionActive: event.status === 'active' }
}

function applyToolCallParts(
  state: AgentRunStreamState,
  event: AgentStreamEvent,
): AgentRunStreamState {
  const parts = event.content?.parts
  if (!parts?.length) {
    return state
  }

  let toolCalls = state.toolCalls
  for (const part of parts) {
    if (part.toolCall) {
      const { id, name, args } = part.toolCall
      toolCalls = [
        ...toolCalls,
        { id, name, args: (args as Record<string, unknown> | undefined) ?? {} },
      ]
    } else if (part.toolResult) {
      const { id, response } = part.toolResult
      toolCalls = toolCalls.map((entry) =>
        entry.id === id
          ? { ...entry, response: (response as Record<string, unknown> | undefined) ?? {} }
          : entry,
      )
    }
  }

  if (toolCalls === state.toolCalls) {
    return state
  }
  return { ...state, toolCalls }
}

function applyAgent(state: AgentRunStreamState, event: AgentStreamEvent): AgentRunStreamState {
  // Process tool call/result parts first
  const next = applyToolCallParts(state, event)

  const delta = snapshotTextFromAgentEvent(event)
  if (delta === undefined) {
    return next
  }

  // Non-partial events after incremental chunks often carry the full aggregated answer; appending
  // would duplicate (mirrors runtime/internal shouldSkipStreamingEvent for streaming consumers).
  // Still append when `partial === false` is a true continuation chunk (suffix, not a superset).
  if (event.partial === false && next.assistantTurnText.length > 0) {
    if (delta === next.assistantTurnText) {
      return next
    }
    if (delta.startsWith(next.assistantTurnText)) {
      return { ...next, assistantTurnText: delta }
    }
  }

  return { ...next, assistantTurnText: next.assistantTurnText + delta }
}

function applyError(state: AgentRunStreamState, event: StreamErrorEvent): AgentRunStreamState {
  return { ...state, busy: false, error: event.message }
}

function applyDone(state: AgentRunStreamState): AgentRunStreamState {
  return { ...state, busy: false }
}

/** One row in the chat transcript (UI uses `assistant`; SSE uses `model`). */
export type ChatTranscriptMessage = {
  role: 'user' | 'assistant'
  text: string
  toolCalls?: ToolCallEntry[]
}

function flushAssistantMessage(state: AgentRunStreamState): ChatTranscriptMessage | null {
  const text = state.assistantTurnText
  const toolCalls = state.toolCalls
  if (!text && toolCalls.length === 0) {
    return null
  }
  const row: ChatTranscriptMessage = { role: 'assistant', text }
  if (toolCalls.length > 0) {
    row.toolCalls = toolCalls
  }
  return row
}

/**
 * Folds one historical `agent` event from an idle `readSession` stream into scratch assistant state
 * and optional transcript rows. Uses the same {@link applyAgent} accumulation rules as live runs;
 * flushes on `turnComplete` or before a user event (role transition).
 */
function toolCallsStillPending(row: ChatTranscriptMessage): boolean {
  return row.toolCalls?.some((tc) => tc.response === undefined) ?? false
}

export function applyIdleHistoryAgentEvent(
  state: AgentRunStreamState,
  event: AgentStreamEvent,
): { state: AgentRunStreamState; appended: ChatTranscriptMessage[] } {
  const appended: ChatTranscriptMessage[] = []

  // ADK emits tool results as `agent` events with role=user; merge them before flushing transcript.
  if (event.content?.role === 'user') {
    let next = applyAgentStreamEvent(state, event)
    const userText = event.content.parts?.map((p) => p.text).join('') ?? ''
    const assistantRow = flushAssistantMessage(next)
    if (assistantRow && !toolCallsStillPending(assistantRow)) {
      appended.push(assistantRow)
      next = { ...next, assistantTurnText: '', toolCalls: [] }
    }
    if (userText) {
      appended.push({ role: 'user', text: userText })
    }
    return { state: next, appended }
  }

  let next = applyAgentStreamEvent(state, event)
  if (event.turnComplete) {
    const row = flushAssistantMessage(next)
    if (row && !toolCallsStillPending(row)) {
      appended.push(row)
      next = { ...next, assistantTurnText: '', toolCalls: [] }
    }
  }
  return { state: next, appended }
}

/**
 * Pure reducer for stream events. `agent` events append text; other kinds update fields as implemented.
 */
export function applyAgentStreamEvent(
  state: AgentRunStreamState,
  event: StreamEvent,
): AgentRunStreamState {
  switch (event.event) {
    case 'sessionBound':
      return applySessionBound(state, event)
    case 'sessionStatus':
      return applySessionStatus(state, event)
    case 'agent':
      return applyAgent(state, event)
    case 'error':
      return applyError(state, event)
    case 'done':
      return applyDone(state)
  }
}
