import { describe, it, expect, beforeEach } from 'vitest'
import { faker } from '@faker-js/faker'
import { parseAgentSseJsonStream, type SseJsonEvent } from './sse'
import { buildAgentRunSseSampleStream } from './testFixtures'

function bytesStream(s: string): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(s))
      controller.close()
    },
  })
}

function chunkedBytesStream(s: string, chunkSize: number): ReadableStream<Uint8Array> {
  const bytes = new TextEncoder().encode(s)
  let offset = 0
  return new ReadableStream({
    pull(controller) {
      if (offset >= bytes.length) {
        controller.close()
        return
      }
      const end = Math.min(offset + chunkSize, bytes.length)
      controller.enqueue(bytes.slice(offset, end))
      offset = end
    },
  })
}

async function collect(stream: ReadableStream<Uint8Array>) {
  const out: SseJsonEvent[] = []
  for await (const ev of parseAgentSseJsonStream(stream)) {
    out.push(ev)
  }
  return out
}

describe('parseAgentSseJsonStream', () => {
  describe('agent run sample stream', () => {
    let sessionId: string
    let partialText: string
    let fullText: string
    let sampleStream: string

    beforeEach(() => {
      sessionId = faker.string.uuid()
      partialText = faker.lorem.word()
      fullText = `${partialText}, ${faker.lorem.words(2)}`
      sampleStream = buildAgentRunSseSampleStream({ sessionId, partialText, fullText })
    })

    it('parses sample stream in one chunk', async () => {
      const events = await collect(bytesStream(sampleStream))
      expect(events).toHaveLength(4)

      expect(events[0]).toEqual({
        eventName: 'sessionBound',
        payload: {
          event: 'sessionBound',
          sessionId,
        },
      })

      expect(events[1]).toEqual({
        eventName: 'agent',
        payload: {
          event: 'agent',
          partial: true,
          content: { parts: [{ text: partialText }] },
        },
      })

      const secondChunk = fullText.slice(partialText.length)
      expect(events[2]).toEqual({
        eventName: 'agent',
        payload: {
          event: 'agent',
          partial: false,
          turnComplete: true,
          content: { parts: [{ text: secondChunk }] },
        },
      })

      expect(events[3]).toEqual({
        eventName: 'done',
        payload: { event: 'done' },
      })
    })

    it('parses the same stream with one byte per chunk (TCP-style splits)', async () => {
      const events = await collect(chunkedBytesStream(sampleStream, 1))
      expect(events).toHaveLength(4)
      const first = events[0]?.payload
      expect(first?.event).toBe('sessionBound')
      if (first?.event === 'sessionBound') {
        expect(first.sessionId).toBe(sessionId)
      }
      expect(events[3]?.payload.event).toBe('done')
    })

    it('accepts CRLF line endings', async () => {
      const crlf = sampleStream.replace(/\n/g, '\r\n')
      const events = await collect(bytesStream(crlf))
      expect(events).toHaveLength(4)
    })

    it('aborts when AbortSignal is aborted', async () => {
      const ac = new AbortController()
      const stream = chunkedBytesStream(sampleStream, 1)
      const iter = parseAgentSseJsonStream(stream, { signal: ac.signal })
      await iter.next()
      ac.abort()
      await expect(iter.next()).rejects.toBeDefined()
    })
  })

  it('joins multiple data: lines with newline before JSON parse', async () => {
    const sse = `event: msg
data: {"event":"agent","partial":
data: true}

`
    const events = await collect(bytesStream(sse))
    expect(events).toHaveLength(1)
    expect(events[0]?.payload).toEqual({
      event: 'agent',
      partial: true,
    })
  })

  it('ignores comment lines starting with colon', async () => {
    const sse = `: ping
event: x
data: {"event":"error","message":"note"}

`
    const events = await collect(bytesStream(sse))
    expect(events).toHaveLength(1)
    expect(events[0]?.payload).toEqual({ event: 'error', message: 'note' })
  })

  it('throws on invalid JSON in data', async () => {
    const sse = `event: bad
data: not-json
`
    const iter = parseAgentSseJsonStream(bytesStream(sse))
    await expect(iter.next()).rejects.toThrow(SyntaxError)
  })

  it('skips event blocks with no data lines', async () => {
    const sse = `event: ping\n\n`
    const events = await collect(bytesStream(sse))
    expect(events).toHaveLength(0)
  })

  it('parses trailing frame when stream closes mid-chunk', async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('data: {"event":"done"}'))
        controller.enqueue(new TextEncoder().encode('\n\n'))
        controller.close()
      },
    })
    const events = await collect(stream)
    expect(events).toHaveLength(1)
    expect(events[0]?.payload).toEqual({ event: 'done' })
  })
})
