/**
 * Builds SSE text for an agent run that includes a tool call and its response.
 * Emits: sessionBound → agent (toolCall part) → agent (toolResult part) → done.
 */
export function buildAgentRunWithToolCallSseStream({
  sessionId,
  toolCallId,
  toolName,
  toolArgs,
  toolResponse,
}: {
  sessionId: string
  toolCallId: string
  toolName: string
  toolArgs: Record<string, unknown>
  toolResponse: Record<string, unknown>
}): string {
  const block = (eventName: string, body: object) =>
    `event: ${eventName}\ndata: ${JSON.stringify(body)}\n\n`
  return [
    block('sessionBound', { event: 'sessionBound', sessionId }),
    block('agent', {
      event: 'agent',
      partial: false,
      content: {
        parts: [{ toolCall: { id: toolCallId, name: toolName, args: toolArgs } }],
      },
    }),
    block('agent', {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: {
        parts: [{ toolResult: { id: toolCallId, name: toolName, response: toolResponse } }],
      },
    }),
    block('done', { event: 'done' }),
  ].join('')
}

/**
 * Builds SSE text for the agent run stream shape used in Vitest (sessionBound → agent → agent → done).
 * `partialText` is the first incremental chunk; `fullText` is the full assistant reply after both chunks
 * are concatenated (second SSE event sends `fullText.slice(partialText.length)`).
 */
export function buildAgentRunSseSampleStream({
  sessionId,
  partialText,
  fullText,
}: {
  sessionId: string
  partialText: string
  fullText: string
}): string {
  const block = (eventName: string, body: object) =>
    `event: ${eventName}\ndata: ${JSON.stringify(body)}\n\n`
  const secondChunk = fullText.slice(partialText.length)
  return [
    block('sessionBound', { event: 'sessionBound', sessionId }),
    block('agent', {
      event: 'agent',
      partial: true,
      content: { parts: [{ text: partialText }] },
    }),
    block('agent', {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { parts: [{ text: secondChunk }] },
    }),
    block('done', { event: 'done' }),
  ].join('')
}

/**
 * Builds SSE text for the readSession stream shape:
 * sessionBound → sessionStatus → agent (history) events → done.
 * `status` controls whether the session is active or idle.
 * `historyMessages` are completed assistant turns to replay (model role, one event per message).
 * `historyTurns` overrides `historyMessages` when set: explicit `user` / `model` roles and optional
 * `partial` / `turnComplete` for streaming-shaped history.
 * When `status` is 'active', extra live agent chunks + done are appended.
 */
export function buildReadSessionSseSampleStream({
  sessionId,
  status,
  historyMessages = [],
  historyTurns,
  liveText,
}: {
  sessionId: string
  status: 'active' | 'idle'
  historyMessages?: string[]
  historyTurns?: Array<{
    role: 'user' | 'model'
    text: string
    partial?: boolean
    turnComplete?: boolean
  }>
  liveText?: string
}): string {
  const block = (eventName: string, body: object) =>
    `event: ${eventName}\ndata: ${JSON.stringify(body)}\n\n`
  const historyBlocks =
    historyTurns?.map((t) =>
      block('agent', {
        event: 'agent',
        partial: t.partial ?? false,
        turnComplete: t.turnComplete ?? true,
        content: { role: t.role, parts: [{ text: t.text }] },
      }),
    ) ??
    historyMessages.map((text) =>
      block('agent', {
        event: 'agent',
        partial: false,
        turnComplete: true,
        content: { parts: [{ text }] },
      }),
    )
  const parts: string[] = [
    block('sessionBound', { event: 'sessionBound', sessionId }),
    block('sessionStatus', { event: 'sessionStatus', status }),
    ...historyBlocks,
  ]
  if (status === 'active' && liveText) {
    parts.push(
      block('agent', {
        event: 'agent',
        partial: true,
        content: { parts: [{ text: liveText }] },
      }),
    )
  }
  parts.push(block('done', { event: 'done' }))
  return parts.join('')
}
