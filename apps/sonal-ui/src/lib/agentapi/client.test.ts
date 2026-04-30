import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { faker } from '@faker-js/faker'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import {
  createAgentApiClient,
  createSonalAgentApi,
} from './client'
import { parseAgentSseJsonStream } from './sse'
import { buildAgentRunSseSampleStream } from './testFixtures'
import type { AgentRunRequest, CreateProviderRequest, ProviderResponse, UpdateProviderRequest } from './types'

describe('createSonalAgentApi / streaming and providers (MSW)', () => {
  function makeAgentRunSampleFixture(): {
    sampleBody: AgentRunRequest
    sampleStream: string
    sessionIdForStream: string
  } {
    const sampleBody = {
      model: `${faker.word.noun().toLowerCase()}/${faker.word.noun().toLowerCase()}`,
      message: { parts: [{ text: faker.lorem.sentence() }] },
    } satisfies AgentRunRequest
    const sessionIdForStream = faker.string.uuid()
    const partialText = faker.lorem.word()
    const fullText = `${partialText}, ${faker.lorem.words(2)}`
    const sampleStream = buildAgentRunSseSampleStream({
      sessionId: sessionIdForStream,
      partialText,
      fullText,
    })
    return { sampleBody, sampleStream, sessionIdForStream }
  }

  // MSW must listen once so Node fetch is intercepted; resetHandlers clears
  // per-test server.use() overrides; close removes interceptors after the suite.
  const server = setupServer()

  beforeAll(() => {
    server.listen({ onUnhandledRequest: 'error' })
  })
  afterEach(() => {
    server.resetHandlers()
  })
  afterAll(() => {
    server.close()
  })

  describe('happy path', () => {
    it('startAgentRun: POSTs JSON with Authorization header, SSE body parses', async () => {
      const { sampleBody, sampleStream, sessionIdForStream } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const accessToken = faker.string.alphanumeric(32)
      const controller = new AbortController()
      let capturedBody: unknown
      let capturedAuthHeader = ''
      server.use(
        http.post(`${baseUrl}/agent-runs`, async ({ request }) => {
          capturedBody = await request.json()
          capturedAuthHeader = request.headers.get('Authorization') ?? ''
          // Undici/MSW may not preserve the same AbortSignal reference as the one passed to
          // fetch; assert the request carries a live signal (forwarded from startAgentRun).
          expect(request.signal.aborted).toBe(false)
          return HttpResponse.text(sampleStream, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          })
        }),
      )
      const api = createSonalAgentApi({ baseUrl, accessToken })
      const res = await api.startAgentRun({
        body: sampleBody,
        signal: controller.signal,
      })
      expect(capturedBody).toEqual(sampleBody)
      expect(capturedAuthHeader).toBe(`Bearer ${accessToken}`)
      expect(res.ok).toBe(true)
      expect(res.body).not.toBeNull()
      const payloads: unknown[] = []
      for await (const ev of parseAgentSseJsonStream(res.body!)) {
        payloads.push(ev.payload)
      }
      expect(payloads).toHaveLength(4)
      expect(payloads[0]).toEqual({
        event: 'sessionBound',
        sessionId: sessionIdForStream,
      })
      expect((payloads[3] as { event: string }).event).toBe('done')
    })

    it('readSession: GETs /sessions/{sessionId} with Authorization header and returns SSE stream', async () => {
      const { sampleStream, sessionIdForStream } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const accessToken = faker.string.alphanumeric(32)
      const rawSessionId = faker.string.uuid()
      let capturedUrl = ''
      let capturedAuthHeader = ''
      server.use(
        http.get(`${baseUrl}/sessions/:sessionId`, ({ request }) => {
          capturedUrl = request.url
          capturedAuthHeader = request.headers.get('Authorization') ?? ''
          return HttpResponse.text(sampleStream, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          })
        }),
      )
      const api = createSonalAgentApi({ baseUrl, accessToken })
      const res = await api.readSession({ sessionId: rawSessionId })
      expect(capturedUrl).toBe(`${baseUrl}/sessions/${rawSessionId}`)
      expect(capturedAuthHeader).toBe(`Bearer ${accessToken}`)
      expect(res.ok).toBe(true)
      expect(res.body).not.toBeNull()
      const payloads: unknown[] = []
      for await (const ev of parseAgentSseJsonStream(res.body!)) {
        payloads.push(ev.payload)
      }
      expect(payloads).toHaveLength(4)
      expect((payloads[0] as { event: string; sessionId: string }).sessionId).toBe(
        sessionIdForStream,
      )
    })

    it('continueAgentRun: POSTs to encoded session URL with body, SSE stream parses', async () => {
      const { sampleBody, sampleStream } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const rawSessionId = `${faker.string.alpha(4)}/${faker.string.alpha(4)} ${faker.string.alpha(3)}`
      let requestUrl = ''
      server.use(
        http.post(`${baseUrl}/sessions/:sessionId/agent-runs`, async ({ request }) => {
          requestUrl = request.url
          const body = await request.json()
          expect(body).toEqual(sampleBody)
          return HttpResponse.text(sampleStream, {
            status: 200,
            headers: { 'Content-Type': 'text/event-stream' },
          })
        }),
      )
      const api = createSonalAgentApi({ baseUrl })
      const res = await api.continueAgentRun({
        sessionId: rawSessionId,
        body: sampleBody,
      })
      expect(requestUrl).toBe(
        `${baseUrl}/sessions/${encodeURIComponent(rawSessionId)}/agent-runs`,
      )
      expect(res.ok).toBe(true)
      expect(res.body).not.toBeNull()
      const payloads: unknown[] = []
      for await (const ev of parseAgentSseJsonStream(res.body!)) {
        payloads.push(ev.payload)
      }
      expect(payloads).toHaveLength(4)
    })
  })

  describe('provider API functions (MSW)', () => {
    function makeProviderFixture(): ProviderResponse {
      return {
        name: faker.word.noun().toLowerCase(),
        type: 'openai-compatible',
        displayName: faker.company.name(),
        baseUrl: faker.internet.url(),
        apiKeyPreview: `...${faker.string.alphanumeric(4)}`,
        models: [],
        createdAt: faker.date.recent().toISOString(),
        updatedAt: faker.date.recent().toISOString(),
      }
    }

    it('listProviders: returns provider list', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const providers = [makeProviderFixture(), makeProviderFixture()]
      server.use(
        http.get(`${baseUrl}/providers`, () =>
          HttpResponse.json({ providers }, { status: 200 }),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const result = await api.listProviders()
      expect(result.providers).toEqual(providers)
    })

    it('createProvider: sends correct request and returns response', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const provider = makeProviderFixture()
      const body: CreateProviderRequest = {
        name: provider.name,
        type: 'openai-compatible',
        baseUrl: provider.baseUrl,
        apiKey: faker.string.alphanumeric(32),
        displayName: provider.displayName,
      }
      let capturedBody: unknown
      server.use(
        http.post(`${baseUrl}/providers`, async ({ request }) => {
          capturedBody = await request.json()
          return HttpResponse.json(provider, { status: 201 })
        }),
      )
      const api = createSonalAgentApi({ baseUrl })
      const result = await api.createProvider({ body })
      expect(capturedBody).toEqual(body)
      expect(result).toEqual(provider)
    })

    it('getProvider: returns single provider', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const provider = makeProviderFixture()
      server.use(
        http.get(`${baseUrl}/providers/:providerName`, () =>
          HttpResponse.json(provider, { status: 200 }),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const result = await api.getProvider({ providerName: provider.name })
      expect(result).toEqual(provider)
    })

    it('updateProvider: sends correct request and returns updated provider', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const provider = makeProviderFixture()
      const body: UpdateProviderRequest = {
        baseUrl: faker.internet.url(),
        displayName: faker.company.name(),
      }
      let capturedBody: unknown
      server.use(
        http.put(`${baseUrl}/providers/:providerName`, async ({ request }) => {
          capturedBody = await request.json()
          return HttpResponse.json({ ...provider, ...body }, { status: 200 })
        }),
      )
      const api = createSonalAgentApi({ baseUrl })
      await api.updateProvider({ providerName: provider.name, body })
      expect(capturedBody).toEqual(body)
    })

    it('deleteProvider: sends DELETE request and resolves', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const providerName = faker.word.noun().toLowerCase()
      let requestUrl = ''
      server.use(
        http.delete(`${baseUrl}/providers/:providerName`, ({ request }) => {
          requestUrl = request.url
          return new HttpResponse(null, { status: 204 })
        }),
      )
      const api = createSonalAgentApi({ baseUrl })
      await api.deleteProvider({ providerName })
      expect(requestUrl).toBe(`${baseUrl}/providers/${providerName}`)
    })
  })

  describe('listSessions (MSW)', () => {
    it('returns session list on success', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const sessionId = faker.string.uuid()
      const sessions = [
        {
          sessionId,
          title: faker.lorem.words(3),
          createdAt: faker.date.past().toISOString(),
          updatedAt: faker.date.recent().toISOString(),
        },
      ]
      const responseBody = {
        sessions,
        total: sessions.length,
      }
      let capturedUrl = ''
      server.use(
        http.get(`${baseUrl}/sessions`, ({ request }) => {
          capturedUrl = request.url
          return HttpResponse.json(responseBody, { status: 200 })
        }),
      )
      const api = createSonalAgentApi({ baseUrl })
      const limit = faker.number.int({ min: 1, max: 100 })
      const offset = faker.number.int({ min: 0, max: 99 })
      const result = await api.listSessions({ limit, offset })
      expect(result).toEqual(responseBody)
      const url = new URL(capturedUrl)
      expect(url.searchParams.get('limit')).toBe(String(limit))
      expect(url.searchParams.get('offset')).toBe(String(offset))

      const limitNoOffset = faker.number.int({ min: 1, max: 100 })
      await api.listSessions({ limit: limitNoOffset })
      const urlNoOffset = new URL(capturedUrl)
      expect(urlNoOffset.searchParams.get('limit')).toBe(String(limitNoOffset))
      expect(urlNoOffset.searchParams.has('offset')).toBe(false)
    })

    it('throws on API error', async () => {
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      server.use(
        http.get(`${baseUrl}/sessions`, () =>
          HttpResponse.json(
            { title: 'Unauthorized', status: 401 },
            {
              status: 401,
              headers: { 'Content-Type': 'application/problem+json' },
            },
          ),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const limit = faker.number.int({ min: 1, max: 100 })
      await expect(api.listSessions({ limit })).rejects.toThrow(/GET \/sessions/)
    })
  })

  describe('error responses', () => {
    it('400 returns application/problem+json (openapi-fetch parses error; response body consumed)', async () => {
      const { sampleBody } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const detail = faker.lorem.sentence()
      server.use(
        http.post(`${baseUrl}/agent-runs`, () =>
          HttpResponse.json(
            { title: 'Bad Request', status: 400, detail },
            {
              status: 400,
              headers: { 'Content-Type': 'application/problem+json' },
            },
          ),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const res = await api.startAgentRun({ body: sampleBody })
      expect(res.ok).toBe(false)
      expect(res.status).toBe(400)

      const client = createAgentApiClient({ baseUrl })
      const { error, response } = await client.POST('/agent-runs', {
        body: sampleBody,
        parseAs: 'stream',
      })
      expect(response.status).toBe(400)
      expect(error).toMatchObject({
        status: 400,
        title: 'Bad Request',
        detail,
      })
    })

    it('401 returns problem details via openapi-fetch error', async () => {
      const { sampleBody } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      server.use(
        http.post(`${baseUrl}/agent-runs`, () =>
          HttpResponse.json(
            { title: 'Unauthorized', status: 401 },
            {
              status: 401,
              headers: { 'Content-Type': 'application/problem+json' },
            },
          ),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const res = await api.startAgentRun({ body: sampleBody })
      expect(res.status).toBe(401)

      const client = createAgentApiClient({ baseUrl })
      const { error } = await client.POST('/agent-runs', {
        body: sampleBody,
        parseAs: 'stream',
      })
      expect(error).toMatchObject({ status: 401, title: 'Unauthorized' })
    })

    it('continue: 404 returns problem details via openapi-fetch error', async () => {
      const { sampleBody } = makeAgentRunSampleFixture()
      const port = faker.internet.port()
      const baseUrl = `http://127.0.0.1:${port}`
      const sid = faker.string.uuid()
      server.use(
        http.post(`${baseUrl}/sessions/:sessionId/agent-runs`, () =>
          HttpResponse.json(
            { title: 'Not Found', status: 404, detail: 'session' },
            {
              status: 404,
              headers: { 'Content-Type': 'application/problem+json' },
            },
          ),
        ),
      )
      const api = createSonalAgentApi({ baseUrl })
      const res = await api.continueAgentRun({
        sessionId: sid,
        body: sampleBody,
      })
      expect(res.status).toBe(404)

      const client = createAgentApiClient({ baseUrl })
      const { error } = await client.POST('/sessions/{sessionId}/agent-runs', {
        params: { path: { sessionId: sid } },
        body: sampleBody,
        parseAs: 'stream',
      })
      expect(error).toMatchObject({ status: 404, title: 'Not Found' })
    })
  })
})
