/**
 * Typed OpenAPI client for agent run HTTP endpoints. Callers pipe {@link Response.body}
 * into {@link parseAgentSseJsonStream} from `./sse`. SSE 200 bodies are not JSON-parsed by
 * openapi-fetch — we use `parseAs: 'stream'` and return the raw {@link Response} for streaming.
 */

import createClient from 'openapi-fetch'
import type { paths } from './agentapi.generated'
import type {
  AgentRunRequest,
  CreateProviderRequest,
  ModelListResponse,
  ProviderListResponse,
  ProviderResponse,
  SessionListResponse,
  UpdateProviderRequest,
} from './types'

export type { paths }

/** Low-level openapi-fetch client; optional Bearer token. Exported for tests that assert on raw openapi-fetch behavior. */
export function createAgentApiClient(params: { baseUrl: string; accessToken?: string | null }) {
  const headers: Record<string, string> = {}
  if (params.accessToken) {
    headers['Authorization'] = `Bearer ${params.accessToken}`
  }
  return createClient<paths>({ baseUrl: params.baseUrl, headers })
}

function throwJsonApiError(operation: string, error: unknown): never {
  const suffix =
    error !== undefined && error !== null && typeof error === 'object'
      ? JSON.stringify(error)
      : String(error)
  throw new Error(`Agent API ${operation} failed: ${suffix}`)
}

/** Pre-configured Agent API (one `openapi-fetch` client per factory call). */
export interface SonalAgentApi {
  startAgentRun(params: { body: AgentRunRequest; signal?: AbortSignal }): Promise<Response>
  continueAgentRun(params: {
    sessionId: string
    body: AgentRunRequest
    signal?: AbortSignal
  }): Promise<Response>
  readSession(params: { sessionId: string; signal?: AbortSignal }): Promise<Response>
  listProviders(): Promise<ProviderListResponse>
  createProvider(params: { body: CreateProviderRequest }): Promise<ProviderResponse>
  getProvider(params: { providerName: string }): Promise<ProviderResponse>
  updateProvider(params: {
    providerName: string
    body: UpdateProviderRequest
  }): Promise<ProviderResponse>
  deleteProvider(params: { providerName: string }): Promise<void>
  listModels(): Promise<ModelListResponse>
  listSessions(params: { limit: number; offset?: number }): Promise<SessionListResponse>
}

export function createSonalAgentApi(params: {
  baseUrl: string
  accessToken?: string | null
}): SonalAgentApi {
  const client = createAgentApiClient(params)

  return {
    async startAgentRun({ body, signal }) {
      const { response } = await client.POST('/agent-runs', {
        body,
        signal,
        parseAs: 'stream',
      })
      return response
    },

    async continueAgentRun({ sessionId, body, signal }) {
      const { response } = await client.POST('/sessions/{sessionId}/agent-runs', {
        params: { path: { sessionId } },
        body,
        signal,
        parseAs: 'stream',
      })
      return response
    },

    async readSession({ sessionId, signal }) {
      const { response } = await client.GET('/sessions/{sessionId}', {
        params: { path: { sessionId } },
        signal,
        parseAs: 'stream',
      })
      return response
    },

    async listProviders() {
      const { data, error } = await client.GET('/providers')
      if (error) throwJsonApiError('GET /providers', error)
      return data!
    },

    async createProvider({ body }) {
      const { data, error } = await client.POST('/providers', { body })
      if (error) throwJsonApiError('POST /providers', error)
      return data!
    },

    async getProvider({ providerName }) {
      const { data, error } = await client.GET('/providers/{providerName}', {
        params: { path: { providerName } },
      })
      if (error) throwJsonApiError('GET /providers/{providerName}', error)
      return data!
    },

    async updateProvider({ providerName, body }) {
      const { data, error } = await client.PUT('/providers/{providerName}', {
        params: { path: { providerName } },
        body,
      })
      if (error) throwJsonApiError('PUT /providers/{providerName}', error)
      return data!
    },

    async deleteProvider({ providerName }) {
      const { error } = await client.DELETE('/providers/{providerName}', {
        params: { path: { providerName } },
      })
      if (error) throwJsonApiError('DELETE /providers/{providerName}', error)
    },

    async listModels() {
      const { data, error } = await client.GET('/models')
      if (error) throwJsonApiError('GET /models', error)
      return data!
    },

    async listSessions({ limit, offset }) {
      const { data, error } = await client.GET('/sessions', {
        params: {
          query:
            offset === undefined
              ? { limit }
              : { limit, offset },
        },
      })
      if (error) throwJsonApiError('GET /sessions', error)
      return data!
    },
  }
}
