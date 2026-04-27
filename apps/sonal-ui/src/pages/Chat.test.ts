import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { faker } from '@faker-js/faker'
import { render, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Chat from './Chat.svelte'
import {
  buildAgentRunSseSampleStream,
  buildAgentRunWithToolCallSseStream,
  buildReadSessionSseSampleStream,
} from '../lib/agentapi/testFixtures'
import type { ModelInfo } from '../lib/agentapi/types'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  push: vi.fn(),
  listModels: vi.fn(),
  listSessions: vi.fn(),
}))

vi.mock('svelte-spa-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('svelte-spa-router')>()
  return {
    ...actual,
    replace: mocks.replace,
    push: mocks.push,
  }
})

vi.mock('../lib/agentapi/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../lib/agentapi/client')>()
  return {
    ...actual,
    createSonalAgentApi: vi.fn((params: Parameters<typeof actual.createSonalAgentApi>[0]) => {
      const api = actual.createSonalAgentApi(params)
      return { ...api, listModels: mocks.listModels, listSessions: mocks.listSessions }
    }),
  }
})

function makeModelInfo(overrides?: Partial<ModelInfo>): ModelInfo {
  return {
    provider: faker.word.noun().toLowerCase(),
    name: faker.word.noun().toLowerCase(),
    displayName: faker.company.name(),
    ...overrides,
  }
}

async function waitForModelPickerReady() {
  await waitFor(() => {
    expect(screen.getByRole('combobox', { name: 'Model' })).toBeInTheDocument()
  })
}

function bytesStream(s: string): ReadableStream<Uint8Array> {
  return new ReadableStream({
    start(controller) {
      controller.enqueue(new TextEncoder().encode(s))
      controller.close()
    },
  })
}

describe('Chat', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
    mocks.replace.mockReset()
    mocks.push.mockReset()
    mocks.listModels.mockReset()
    mocks.listModels.mockResolvedValue({ models: [makeModelInfo()] })
    mocks.listSessions.mockReset()
    mocks.listSessions.mockResolvedValue({ sessions: [], total: 0 })
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders heading and shows composer on /chat without session', () => {
    render(Chat, { props: { params: {} } })
    expect(screen.getByRole('heading', { name: 'Chat' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Message' })).toBeInTheDocument()
  })

  it('shows composer when sessionId is in the route', () => {
    render(Chat, { props: { params: { sessionId: faker.string.uuid() } } })
    expect(screen.getByRole('textbox', { name: 'Message' })).toBeInTheDocument()
  })

  it('submits a new session, streams assistant text, and binds session in the URL', async () => {
    const sessionId = faker.string.uuid()
    const partialText = faker.lorem.word()
    const fullText = `${partialText}, ${faker.lorem.words(2)}`
    const sse = buildAgentRunSseSampleStream({ sessionId, partialText, fullText })
    const fetchMock = vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    const msg = faker.lorem.sentence()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), msg)
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const firstInput = fetchMock.mock.calls[0][0]
    const requestUrl = typeof firstInput === 'string' ? firstInput : (firstInput as Request).url
    expect(requestUrl).toMatch(/\/agent-runs$/)

    await waitFor(() => {
      expect(screen.getByText(msg)).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText(fullText)).toBeInTheDocument()
    })
    expect(mocks.replace).toHaveBeenCalledWith(
      `/chat/${encodeURIComponent(sessionId)}`,
    )
    // Only the POST stream; URL sync must not trigger readSession (GET /sessions/{id}).
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('does not abort the agent run POST when the URL gains sessionId (hydration is separate)', async () => {
    const sessionId = faker.string.uuid()
    const partialText = faker.lorem.word()
    const fullText = `${partialText}, ${faker.lorem.words(2)}`
    const runSse = buildAgentRunSseSampleStream({ sessionId, partialText, fullText })
    const readSse = buildReadSessionSseSampleStream({ sessionId, status: 'idle' })

    let postAbortSignal: AbortSignal | undefined

    vi.mocked(globalThis.fetch).mockImplementation((input, init) => {
      const url = typeof input === 'string' ? input : (input as Request).url
      if (url.includes('/sessions/') && !url.includes('/agent-runs')) {
        return Promise.resolve(
          new Response(bytesStream(readSse), {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          }),
        )
      }
      const signal =
        init?.signal ??
        (typeof input !== 'string' && 'signal' in input ? input.signal : undefined)
      postAbortSignal = signal ?? undefined
      return Promise.resolve(
        new Response(bytesStream(runSse), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      )
    })

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), faker.lorem.sentence())
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => {
      expect(mocks.replace).toHaveBeenCalledWith(`/chat/${encodeURIComponent(sessionId)}`)
    })
    expect(postAbortSignal?.aborted).toBe(false)

    await waitFor(() => {
      expect(screen.getByText(fullText)).toBeInTheDocument()
    })
    expect(postAbortSignal?.aborted).toBe(false)
    const readSessionCalls = vi.mocked(globalThis.fetch).mock.calls.filter(([input]) => {
      const url = typeof input === 'string' ? input : (input as Request).url
      return url.includes('/sessions/') && !url.includes('/agent-runs')
    })
    expect(readSessionCalls.length).toBe(0)
  })

  it('POSTs to continue endpoint when sessionId is present', async () => {
    const sessionId = faker.string.uuid()
    const partialText = faker.lorem.word()
    const fullText = `${partialText} full`
    const readSse = buildReadSessionSseSampleStream({ sessionId, status: 'idle' })
    const continueSse = buildAgentRunSseSampleStream({ sessionId, partialText, fullText })
    const fetchMock = vi.mocked(globalThis.fetch)
      .mockResolvedValueOnce(
        new Response(bytesStream(readSse), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(bytesStream(continueSse), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      )

    render(Chat, { props: { params: { sessionId } } })
    const user = userEvent.setup()
    // Wait for readSession to complete so the composer is re-enabled
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Message' })).not.toBeDisabled())
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), faker.lorem.sentence())
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    const secondInput = fetchMock.mock.calls[1][0]
    const requestUrl = typeof secondInput === 'string' ? secondInput : (secondInput as Request).url
    expect(requestUrl).toContain(`/sessions/${encodeURIComponent(sessionId)}/agent-runs`)
  })

  it('shows request error when response is not ok', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(new Response(null, { status: 502 }))

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(
      await screen.findByRole('alert'),
    ).toHaveTextContent('Request failed (502)')
  })

  it('shows error when response has no body', async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(new Response(null, { status: 200 }))

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('No response body')
  })

  it('shows network error message when fetch rejects', async () => {
    vi.mocked(globalThis.fetch).mockRejectedValue(new Error('network down'))

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('network down')
  })

  it('shows generic message when fetch rejects with non-Error', async () => {
    vi.mocked(globalThis.fetch).mockRejectedValue('boom')

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Request failed')
  })

  it('New chat resets transcript and navigates to /chat', async () => {
    const sessionId = faker.string.uuid()
    const sse = buildAgentRunSseSampleStream({
      sessionId,
      partialText: 'a',
      fullText: 'ab',
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'one')
    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(mocks.push).toHaveBeenCalledWith('/chat')
    expect(screen.queryByText('one')).not.toBeInTheDocument()
  })

  it('ignores SSE payloads that are not stream events', async () => {
    const sse = `event: x\ndata: {"foo":1}\n\n`
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => expect(screen.getByText('hi')).toBeInTheDocument())
  })

  it('surfaces stream error events', async () => {
    const sse = `event: error\ndata: ${JSON.stringify({ event: 'error', message: 'stream failed' })}\n\n`
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('stream failed')
    })
  })

  it('shows thinking placeholder while busy without assistant text', async () => {
    const sessionId = faker.string.uuid()
    const sse = `event: sessionBound\ndata: ${JSON.stringify({ event: 'sessionBound', sessionId })}\n\n`
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => {
      expect(screen.getByText('Thinking…')).toBeInTheDocument()
    })
  })

  it('on mount with sessionId, calls readSession (GET /sessions/...)', async () => {
    const sessionId = faker.string.uuid()
    const sse = buildReadSessionSseSampleStream({ sessionId, status: 'idle' })
    const fetchMock = vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: { sessionId } } })

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const firstInput = fetchMock.mock.calls[0][0]
    const requestUrl = typeof firstInput === 'string' ? firstInput : (firstInput as Request).url
    expect(requestUrl).toContain(`/sessions/${encodeURIComponent(sessionId)}`)
    expect(requestUrl).not.toContain('/agent-runs')
  })

  it('idle session: populates messages from history and shows ready composer', async () => {
    const sessionId = faker.string.uuid()
    const historyMsg = faker.lorem.sentence()
    const sse = buildReadSessionSseSampleStream({
      sessionId,
      status: 'idle',
      historyMessages: [historyMsg],
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: { sessionId } } })

    await waitFor(() => {
      expect(screen.getByText(historyMsg)).toBeInTheDocument()
    })
    // Composer is ready (not disabled) for idle session
    expect(screen.getByRole('textbox', { name: 'Message' })).not.toBeDisabled()
  })

  it('idle session: replays user and model history as separate bubbles', async () => {
    const sessionId = faker.string.uuid()
    const userLine = faker.lorem.sentence()
    const assistantLine = faker.lorem.sentence()
    const sse = buildReadSessionSseSampleStream({
      sessionId,
      status: 'idle',
      historyTurns: [
        { role: 'user', text: userLine, turnComplete: true },
        { role: 'model', text: assistantLine, turnComplete: true },
      ],
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: { sessionId } } })

    await waitFor(() => {
      expect(screen.getByText(userLine)).toBeInTheDocument()
      expect(screen.getByText(assistantLine)).toBeInTheDocument()
    })
  })

  it('idle session: model history partial then full does not duplicate prefix', async () => {
    const sessionId = faker.string.uuid()
    const partialText = faker.lorem.word()
    const fullText = `${partialText}, ${faker.lorem.words(2)}`
    const sse = buildReadSessionSseSampleStream({
      sessionId,
      status: 'idle',
      historyTurns: [
        { role: 'model', text: partialText, partial: true, turnComplete: false },
        { role: 'model', text: fullText, partial: false, turnComplete: true },
      ],
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: { sessionId } } })

    await waitFor(() => {
      expect(screen.getByText(fullText)).toBeInTheDocument()
    })
  })

  it('renders tool call block in committed messages after done', async () => {
    const sessionId = faker.string.uuid()
    const toolCallId = faker.string.uuid()
    const toolName = 'workspacefs_write_file'
    const toolArgs = { path: '/tmp/test.txt', content: 'hello' }
    const toolResponse = { success: true }
    const sse = buildAgentRunWithToolCallSseStream({
      sessionId,
      toolCallId,
      toolName,
      toolArgs,
      toolResponse,
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitForModelPickerReady()
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'do something')
    await user.click(screen.getByRole('button', { name: 'Send' }))

    await waitFor(() => {
      expect(screen.getByText(toolName)).toBeInTheDocument()
    })
  })

  it('blocks send when no models and links to Providers', async () => {
    mocks.listModels.mockResolvedValue({ models: [] })
    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    await waitFor(() => {
      expect(screen.getByText(/No models available/i)).toBeInTheDocument()
    })
    const providersLink = screen.getByRole('link', { name: 'Providers' })
    expect(providersLink.getAttribute('href')).toMatch(/providers$/)
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hello')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('blocks send when listModels fails', async () => {
    mocks.listModels.mockRejectedValue(new Error('models unavailable'))
    render(Chat, { props: { params: {} } })
    const user = userEvent.setup()
    expect(await screen.findByRole('alert')).toHaveTextContent('models unavailable')
    await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hi')
    expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled()
  })

  it('active session: shows streaming state while reconnecting', async () => {
    const sessionId = faker.string.uuid()
    const liveText = faker.lorem.word()
    const sse = buildReadSessionSseSampleStream({
      sessionId,
      status: 'active',
      liveText,
    })
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(bytesStream(sse), {
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
      }),
    )

    render(Chat, { props: { params: { sessionId } } })

    await waitFor(() => {
      expect(screen.getByText(liveText)).toBeInTheDocument()
    })
  })

  describe('model picker', () => {
    it('renders model picker with available models when composer is open', async () => {
      const model1 = makeModelInfo()
      const model2 = makeModelInfo()
      mocks.listModels.mockResolvedValue({ models: [model1, model2] })

      render(Chat, { props: { params: {} } })

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: 'Model' })).toBeInTheDocument()
      })
      const select = screen.getByRole('combobox', { name: 'Model' })
      expect(select).toHaveDisplayValue(model1.displayName!)
      const options = screen.getAllByRole('option')
      expect(options).toHaveLength(2)
    })

    it('selecting a model updates the picker state', async () => {
      const model1 = makeModelInfo()
      const model2 = makeModelInfo()
      mocks.listModels.mockResolvedValue({ models: [model1, model2] })

      render(Chat, { props: { params: {} } })
      const user = userEvent.setup()

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: 'Model' })).toBeInTheDocument()
      })
      await user.selectOptions(screen.getByRole('combobox', { name: 'Model' }), `${model2.provider}/${model2.name}`)
      expect(screen.getByRole('combobox', { name: 'Model' })).toHaveValue(`${model2.provider}/${model2.name}`)
    })

    it('submit sends selected model in request body', async () => {
      const model = makeModelInfo()
      mocks.listModels.mockResolvedValue({ models: [model] })
      const sessionId = faker.string.uuid()
      const sse = buildAgentRunSseSampleStream({ sessionId, partialText: 'a', fullText: 'ab' })
      const fetchMock = vi.mocked(globalThis.fetch).mockResolvedValue(
        new Response(bytesStream(sse), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      )

      render(Chat, { props: { params: {} } })
      const user = userEvent.setup()

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: 'Model' })).toBeInTheDocument()
      })
      await user.type(screen.getByRole('textbox', { name: 'Message' }), 'hello')
      await user.click(screen.getByRole('button', { name: 'Send' }))

      await waitFor(() => expect(fetchMock).toHaveBeenCalled())
      const firstArg = fetchMock.mock.calls[0][0]
      const sentBody = await (firstArg as Request).json()
      expect(sentBody.model).toBe(`${model.provider}/${model.name}`)
    })

    it('default selection uses localStorage value if present', async () => {
      const model1 = makeModelInfo()
      const model2 = makeModelInfo()
      const savedModel = `${model2.provider}/${model2.name}`
      localStorage.setItem('selectedModel', savedModel)
      mocks.listModels.mockResolvedValue({ models: [model1, model2] })

      render(Chat, { props: { params: {} } })

      await waitFor(() => {
        expect(screen.getByRole('combobox', { name: 'Model' })).toBeInTheDocument()
      })
      expect(screen.getByRole('combobox', { name: 'Model' })).toHaveValue(savedModel)
    })
  })

  describe('session list sidebar', () => {
    it('shows a Sessions control on narrow viewports', () => {
      vi.stubGlobal(
        'matchMedia',
        vi.fn().mockImplementation(() => ({
          matches: true,
          media: '(max-width: 768px)',
          addEventListener: vi.fn(),
          removeEventListener: vi.fn(),
        })),
      )
      render(Chat, { props: { params: {} } })
      expect(screen.getByRole('button', { name: 'Sessions' })).toBeInTheDocument()
    })

    it('renders session list navigation beside the main column', () => {
      render(Chat, { props: { params: {} } })
      expect(screen.getByRole('navigation', { name: 'Sessions' })).toBeInTheDocument()
      expect(screen.getByRole('complementary', { name: 'Session list' })).toBeInTheDocument()
    })

    it('refreshes the session list after a send completes', async () => {
      const sessionId = faker.string.uuid()
      const partialText = faker.lorem.word()
      const fullText = `${partialText}, ${faker.lorem.words(2)}`
      const sse = buildAgentRunSseSampleStream({ sessionId, partialText, fullText })
      vi.mocked(globalThis.fetch).mockResolvedValue(
        new Response(bytesStream(sse), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' },
        }),
      )

      render(Chat, { props: { params: {} } })
      await waitFor(() => expect(mocks.listSessions).toHaveBeenCalled())
      mocks.listSessions.mockClear()

      const user = userEvent.setup()
      await waitForModelPickerReady()
      await user.type(screen.getByRole('textbox', { name: 'Message' }), faker.lorem.sentence())
      await user.click(screen.getByRole('button', { name: 'Send' }))

      await waitFor(() => expect(mocks.listSessions).toHaveBeenCalled())
    })
  })
})
