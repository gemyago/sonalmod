import { describe, it, expect } from 'vitest'
import { faker } from '@faker-js/faker'
import {
  applyAgentStreamEvent,
  applyIdleHistoryAgentEvent,
  createInitialAgentRunStreamState,
} from './streamState'
import { isStreamEvent } from './types'
import type { StreamEvent } from './types'

function makeToolCallEvent(id: string, name: string, args: Record<string, unknown>) {
  return {
    event: 'agent' as const,
    content: { parts: [{ toolCall: { id, name, args } }] },
  }
}

function makeToolResultEvent(id: string, name: string, response: Record<string, unknown>) {
  return {
    event: 'agent' as const,
    content: { parts: [{ toolResult: { id, name, response } }] },
  }
}

describe('isStreamEvent', () => {
  it('recognizes sessionStatus event', () => {
    expect(isStreamEvent({ event: 'sessionStatus', status: 'active' })).toBe(true)
    expect(isStreamEvent({ event: 'sessionStatus', status: 'idle' })).toBe(true)
  })

  it('rejects unknown event types', () => {
    expect(isStreamEvent({ event: 'unknown' })).toBe(false)
    expect(isStreamEvent(null)).toBe(false)
    expect(isStreamEvent({ noEvent: true })).toBe(false)
  })
})

describe('applyAgentStreamEvent', () => {
  const base = (): ReturnType<typeof createInitialAgentRunStreamState> =>
    createInitialAgentRunStreamState()

  it('sessionBound sets sessionId', () => {
    const sessionId = faker.string.uuid()
    let state = base()
    const ev: StreamEvent = {
      event: 'sessionBound',
      sessionId,
    }
    state = applyAgentStreamEvent(state, ev)
    expect(state.sessionId).toBe(sessionId)
    expect(state.busy).toBe(true)
    expect(state.error).toBeNull()
  })

  it('agent events concatenate successive chunks', () => {
    const first = faker.lorem.word()
    const second = faker.lorem.word()
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: true,
      content: { parts: [{ text: first }] },
    })
    expect(state.assistantTurnText).toBe(first)

    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: true,
      turnComplete: false,
      content: { parts: [{ text: second }] },
    })
    expect(state.assistantTurnText).toBe(first + second)
  })

  it('non-partial final that repeats the full answer does not append again', () => {
    const full = faker.lorem.sentence()
    const prefix = full.slice(0, Math.max(3, Math.floor(full.length / 2)))
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: true,
      content: { parts: [{ text: prefix }] },
    })
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { parts: [{ text: full }] },
    })
    expect(state.assistantTurnText).toBe(full)
  })

  it('agent events with user role do not append to assistantTurnText', () => {
    const userMsg = faker.lorem.sentence()
    const modelMsg = faker.lorem.sentence()
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      content: { parts: [{ text: userMsg }], role: 'user' },
    })
    expect(state.assistantTurnText).toBe('')
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: true,
      content: { parts: [{ text: modelMsg }], role: 'model' },
    })
    expect(state.assistantTurnText).toBe(modelMsg)
  })

  it('non-partial incremental suffix after partials still appends when not a superset', () => {
    const first = faker.lorem.word()
    const rest = `, ${faker.lorem.words(2)}`
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: true,
      content: { parts: [{ text: first }] },
    })
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { parts: [{ text: rest }] },
    })
    expect(state.assistantTurnText).toBe(first + rest)
  })

  it('agent event joins multiple parts into one append', () => {
    const a = faker.string.alphanumeric(4)
    const b = faker.string.alphanumeric(4)
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      content: { parts: [{ text: a }, { text: b }] },
    })
    expect(state.assistantTurnText).toBe(a + b)
  })

  it('agent with no content.parts leaves previous assistantTurnText', () => {
    const keep = faker.lorem.words(3)
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      content: { parts: [{ text: keep }] },
    })
    state = applyAgentStreamEvent(state, { event: 'agent' })
    expect(state.assistantTurnText).toBe(keep)
  })

  it('error sets message, clears busy, and does not clear sessionId', () => {
    const boundId = faker.string.uuid()
    const errMsg = faker.lorem.sentence()
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'sessionBound',
      sessionId: boundId,
    })
    state = applyAgentStreamEvent(state, {
      event: 'error',
      message: errMsg,
    })
    expect(state.error).toBe(errMsg)
    expect(state.busy).toBe(false)
    expect(state.sessionId).toBe(boundId)
  })

  it('done flips busy to false', () => {
    let state = base()
    expect(state.busy).toBe(true)
    state = applyAgentStreamEvent(state, { event: 'done' })
    expect(state.busy).toBe(false)
    expect(state.error).toBeNull()
  })

  it('sessionStatus active sets sessionActive to true', () => {
    const ev: StreamEvent = { event: 'sessionStatus', status: 'active' }
    const state = applyAgentStreamEvent(base(), ev)
    expect(state.sessionActive).toBe(true)
    expect(state.busy).toBe(true)
  })

  it('sessionStatus idle sets sessionActive to false', () => {
    const ev: StreamEvent = { event: 'sessionStatus', status: 'idle' }
    const state = applyAgentStreamEvent(base(), ev)
    expect(state.sessionActive).toBe(false)
  })
})

describe('tool call tracking in applyAgentStreamEvent', () => {
  const base = () => createInitialAgentRunStreamState()

  it('toolCall part adds entry to state.toolCalls', () => {
    const id = faker.string.uuid()
    const name = faker.word.noun()
    const args = { path: faker.system.filePath() }
    let state = base()
    state = applyAgentStreamEvent(state, makeToolCallEvent(id, name, args))
    expect(state.toolCalls).toHaveLength(1)
    expect(state.toolCalls[0]).toEqual({ id, name, args })
    expect(state.assistantTurnText).toBe('')
  })

  it('toolResult part updates matching entry response', () => {
    const id = faker.string.uuid()
    const name = faker.word.noun()
    const args = { path: faker.system.filePath() }
    const response = { result: 'ok' }
    let state = base()
    state = applyAgentStreamEvent(state, makeToolCallEvent(id, name, args))
    state = applyAgentStreamEvent(state, makeToolResultEvent(id, name, response))
    expect(state.toolCalls).toHaveLength(1)
    expect(state.toolCalls[0].response).toEqual(response)
  })

  it('mixed text + toolCall: text accumulated, tool call tracked', () => {
    const text = faker.lorem.words(3)
    const id = faker.string.uuid()
    const name = faker.word.noun()
    const args = { x: 1 }
    let state = base()
    state = applyAgentStreamEvent(state, {
      event: 'agent',
      content: { parts: [{ text }, { toolCall: { id, name, args } }] },
    })
    expect(state.assistantTurnText).toBe(text)
    expect(state.toolCalls).toHaveLength(1)
    expect(state.toolCalls[0].name).toBe(name)
  })

  it('toolCall-only event is still flushable (creates entry with no response)', () => {
    const id = faker.string.uuid()
    const name = faker.word.noun()
    let state = base()
    state = applyAgentStreamEvent(state, makeToolCallEvent(id, name, {}))
    expect(state.toolCalls).toHaveLength(1)
    expect(state.toolCalls[0].response).toBeUndefined()
  })
})

describe('applyIdleHistoryAgentEvent', () => {
  const base = (): ReturnType<typeof createInitialAgentRunStreamState> =>
    createInitialAgentRunStreamState()

  it('emits user then assistant rows in order', () => {
    const userMsg = faker.lorem.sentence()
    const modelMsg = faker.lorem.sentence()
    let state = base()
    let r = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { role: 'user', parts: [{ text: userMsg }] },
    })
    expect(r.appended).toEqual([{ role: 'user', text: userMsg }])
    state = r.state

    r = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { role: 'model', parts: [{ text: modelMsg }] },
    })
    expect(r.appended).toEqual([{ role: 'assistant', text: modelMsg }])
  })

  it('partial then full model does not duplicate prefix in flushed row', () => {
    const full = faker.lorem.sentence()
    const prefix = full.slice(0, Math.max(3, Math.floor(full.length / 2)))
    let state = base()
    state = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      partial: true,
      turnComplete: false,
      content: { role: 'model', parts: [{ text: prefix }] },
    }).state
    const r = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      partial: false,
      turnComplete: true,
      content: { role: 'model', parts: [{ text: full }] },
    })
    expect(r.appended).toEqual([{ role: 'assistant', text: full }])
  })

  it('flushes tool calls with assistant message on turnComplete', () => {
    const id = faker.string.uuid()
    const name = faker.word.noun()
    const args = { key: 'val' }
    const response = { ok: true }
    let state = base()
    state = applyIdleHistoryAgentEvent(state, makeToolCallEvent(id, name, args)).state
    state = applyIdleHistoryAgentEvent(state, makeToolResultEvent(id, name, response)).state

    const r = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      turnComplete: true,
      content: { role: 'model', parts: [{ text: 'done' }] },
    })
    expect(r.appended).toHaveLength(1)
    expect(r.appended[0].toolCalls).toHaveLength(1)
    expect(r.appended[0].toolCalls![0]).toEqual({ id, name, args, response })
    expect(r.state.toolCalls).toHaveLength(0)
  })

  it('merges toolResult when ADK sends role=user (idle session replay)', () => {
    const id = faker.string.uuid()
    const name = faker.word.noun()
    const args = { workspace: 'agent-temp' }
    const response = { stdout: 'a.txt\n', exitCode: 0 }
    let state = base()
    state = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      turnComplete: true,
      partial: false,
      content: { role: 'model', parts: [{ toolCall: { id, name, args } }] },
    }).state
    const r = applyIdleHistoryAgentEvent(state, {
      event: 'agent',
      turnComplete: false,
      partial: false,
      content: { role: 'user', parts: [{ toolResult: { id, name, response } }] },
    })
    expect(r.appended).toHaveLength(1)
    expect(r.appended[0].role).toBe('assistant')
    expect(r.appended[0].toolCalls![0]).toEqual({ id, name, args, response })
    expect(r.state.toolCalls).toHaveLength(0)
  })
})
