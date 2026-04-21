import { describe, it, expect, vi, beforeEach } from 'vitest'
import { faker } from '@faker-js/faker'
import { createAuthFetch } from './auth-fetch'
import { AuthStore } from './auth-store.svelte'

vi.mock('./auth-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth-api')>()
  return { ...actual, refreshApi: vi.fn() }
})

import { refreshApi } from './auth-api'
const mockRefreshApi = vi.mocked(refreshApi)

function makeStore(accessToken: string | null = null): AuthStore {
  const store = new AuthStore()
  if (accessToken) {
    store.accessToken = accessToken
    store.user = { id: faker.string.uuid(), username: faker.internet.username() }
  }
  return store
}

describe('createAuthFetch', () => {
  beforeEach(() => {
    localStorage.clear()
    mockRefreshApi.mockReset()
    vi.unstubAllGlobals()
  })

  it('adds Authorization header when access token is present', async () => {
    const token = faker.string.alphanumeric(32)
    const store = makeStore(token)
    const authFetch = createAuthFetch(store)

    const mockFetch = vi.fn().mockResolvedValue(new Response('ok', { status: 200 }))
    vi.stubGlobal('fetch', mockFetch)

    await authFetch('/api/v1/runtime/test')

    expect(mockFetch).toHaveBeenCalledWith(
      '/api/v1/runtime/test',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: `Bearer ${token}` }),
      }),
    )
  })

  it('does not add Authorization header when no access token', async () => {
    const store = makeStore(null)
    const authFetch = createAuthFetch(store)

    const mockFetch = vi.fn().mockResolvedValue(new Response('ok', { status: 200 }))
    vi.stubGlobal('fetch', mockFetch)

    await authFetch('/api/v1/runtime/test')

    const callArgs = mockFetch.mock.calls[0]
    const headers = (callArgs[1] as RequestInit | undefined)?.headers as
      | Record<string, string>
      | undefined
    expect(headers?.Authorization).toBeUndefined()
  })

  it('retries with new token after 401 if refresh succeeds', async () => {
    const oldToken = faker.string.alphanumeric(32)
    const newToken = faker.string.alphanumeric(32)
    const newRefresh = faker.string.alphanumeric(48)
    const store = makeStore(oldToken)
    const authFetch = createAuthFetch(store)

    const mockFetch = vi
      .fn()
      .mockResolvedValueOnce(new Response('Unauthorized', { status: 401 }))
      .mockResolvedValueOnce(new Response('ok', { status: 200 }))
    vi.stubGlobal('fetch', mockFetch)

    const user = { id: faker.string.uuid(), username: faker.internet.username() }
    mockRefreshApi.mockResolvedValue({ accessToken: newToken, refreshToken: newRefresh, user })
    localStorage.setItem('auth_refresh_token', faker.string.alphanumeric(48))

    const res = await authFetch('/api/v1/runtime/test')

    expect(res.status).toBe(200)
    expect(mockRefreshApi).toHaveBeenCalledOnce()
    expect(store.accessToken).toBe(newToken)
    // Second call uses new token
    const secondCall = mockFetch.mock.calls[1]
    const headers = (secondCall[1] as RequestInit).headers as Record<string, string>
    expect(headers.Authorization).toBe(`Bearer ${newToken}`)
  })

  it('clears auth and returns 401 response when refresh fails after 401', async () => {
    const oldToken = faker.string.alphanumeric(32)
    const store = makeStore(oldToken)
    const authFetch = createAuthFetch(store)

    const mockFetch = vi
      .fn()
      .mockResolvedValue(new Response('Unauthorized', { status: 401 }))
    vi.stubGlobal('fetch', mockFetch)

    mockRefreshApi.mockRejectedValue(new Error('Refresh failed: 401'))
    localStorage.setItem('auth_refresh_token', faker.string.alphanumeric(48))

    const res = await authFetch('/api/v1/runtime/test')

    expect(res.status).toBe(401)
    expect(store.accessToken).toBeNull()
    expect(store.user).toBeNull()
    // No retry after refresh failure
    expect(mockFetch).toHaveBeenCalledOnce()
  })

  it('returns non-401 responses without refresh attempt', async () => {
    const token = faker.string.alphanumeric(32)
    const store = makeStore(token)
    const authFetch = createAuthFetch(store)

    const mockFetch = vi.fn().mockResolvedValue(new Response('Not Found', { status: 404 }))
    vi.stubGlobal('fetch', mockFetch)

    const res = await authFetch('/api/v1/runtime/test')

    expect(res.status).toBe(404)
    expect(mockRefreshApi).not.toHaveBeenCalled()
    expect(mockFetch).toHaveBeenCalledOnce()
  })
})
