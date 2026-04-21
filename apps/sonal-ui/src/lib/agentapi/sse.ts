/**
 * Incremental SSE parser for fetch() response bodies (UTF-8).
 * Yields one JSON value per SSE event (after joining multi-line `data:` fields).
 */

import type { StreamEvent } from './types'

function normalizeNewlines(s: string): string {
  return s.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

function parseEventBlock(block: string): { eventName?: string; data: string } | null {
  const lines = normalizeNewlines(block).split('\n')
  let eventName: string | undefined
  const dataLines: string[] = []

  for (const line of lines) {
    if (line === '' || line.startsWith(':')) {
      continue
    }
    if (line.startsWith('event:')) {
      eventName = line.slice('event:'.length).trimStart()
      continue
    }
    if (line.startsWith('data:')) {
      dataLines.push(line.slice('data:'.length).trimStart())
    }
  }

  if (dataLines.length === 0) {
    return null
  }

  return { eventName, data: dataLines.join('\n') }
}

export interface SseJsonEvent {
  /** SSE `event:` field when present; omitted for unnamed events. */
  eventName?: string
  /** Parsed JSON from the event's `data:` payload(s); agent API frames match {@link StreamEvent}. */
  payload: StreamEvent
}

function toSseJsonEvent(frame: { eventName?: string; data: string }): SseJsonEvent {
  let payload: StreamEvent
  try {
    payload = JSON.parse(frame.data) as StreamEvent
  } catch {
    throw new SyntaxError(`Invalid JSON in SSE data: ${frame.data.slice(0, 120)}`)
  }
  return { eventName: frame.eventName, payload }
}

function* drainCompleteFrames(
  buffer: string,
): Generator<{ kind: 'event'; event: SseJsonEvent } | { kind: 'rest'; rest: string }> {
  let b = buffer
  while (true) {
    b = normalizeNewlines(b)
    const sep = b.indexOf('\n\n')
    if (sep === -1) {
      yield { kind: 'rest', rest: b }
      return
    }
    const block = b.slice(0, sep)
    b = b.slice(sep + 2)
    const parsed = parseEventBlock(block)
    if (parsed) {
      yield { kind: 'event', event: toSseJsonEvent(parsed) }
    }
  }
}

/**
 * Reads an SSE stream and yields one {@link SseJsonEvent} per completed event frame.
 * Handles chunk boundaries splitting mid-line or mid-frame.
 */
export async function* parseAgentSseJsonStream(
  stream: ReadableStream<Uint8Array>,
  options?: { signal?: AbortSignal },
): AsyncGenerator<SseJsonEvent, void, undefined> {
  const signal = options?.signal
  const reader = stream.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const throwIfAborted = () => {
    if (signal?.aborted) {
      throw signal.reason
    }
  }

  try {
    while (true) {
      throwIfAborted()

      for (const step of drainCompleteFrames(buffer)) {
        if (step.kind === 'event') {
          yield step.event
        } else {
          buffer = step.rest
        }
      }

      const { done, value } = await reader.read()
      if (done) {
        buffer += decoder.decode()
        for (const step of drainCompleteFrames(buffer)) {
          if (step.kind === 'event') {
            yield step.event
          } else {
            buffer = step.rest
          }
        }
        break
      }
      buffer += decoder.decode(value, { stream: true })
    }

    const tail = normalizeNewlines(buffer).trimEnd()
    if (tail.length > 0) {
      const parsed = parseEventBlock(tail)
      if (parsed) {
        yield toSseJsonEvent(parsed)
      }
    }
  } finally {
    reader.releaseLock()
  }
}
