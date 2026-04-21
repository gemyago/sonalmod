import { describe, it, expect, beforeEach, vi } from 'vitest'
import { faker } from '@faker-js/faker'
import { AuthStore } from './auth-store.svelte'

// Mock the auth-api module so AuthStore's import of refreshApi is controlled in tests.
vi.mock('./auth-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth-api')>()
  return { ...actual, refreshApi: vi.fn() }
})

import { refreshApi } from './auth-api'
const mockRefreshApi = vi.mocked(refreshApi)

function makeUser() {
  return { id: faker.string.uuid(), username: faker.internet.username() }
}

function makeTokens() {
  return {
    accessToken: faker.string.alphanumeric(32),
    refreshToken: faker.string.alphanumeric(48),
  }
}

const REFRESH_TOKEN_KEY = 'auth_refresh_token'

describe('AuthStore', () => {
  beforeEach(() => {
    localStorage.clear()
    mockRefreshApi.mockReset()
  })

  it('starts unauthenticated with no user or token', () => {
    const store = new AuthStore()
    expect(store.accessToken).toBeNull()
    expect(store.user).toBeNull()
    expect(store.isAuthenticated).toBe(false)
  })

  it('setAuth sets tokens, user, and persists refreshToken to localStorage', () => {
    const store = new AuthStore()
    const user = makeUser()
    const { accessToken, refreshToken } = makeTokens()

    store.setAuth(accessToken, refreshToken, user)

    expect(store.accessToken).toBe(accessToken)
    expect(store.user).toEqual(user)
    expect(store.isAuthenticated).toBe(true)
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe(refreshToken)
  })

  it('clearAuth removes access token, user, and localStorage entry', () => {
    const store = new AuthStore()
    const user = makeUser()
    const { accessToken, refreshToken } = makeTokens()

    store.setAuth(accessToken, refreshToken, user)
    store.clearAuth()

    expect(store.accessToken).toBeNull()
    expect(store.user).toBeNull()
    expect(store.isAuthenticated).toBe(false)
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBeNull()
  })

  it('tryRestoreSession does nothing when no refreshToken in localStorage', async () => {
    const store = new AuthStore()
    await store.tryRestoreSession()
    expect(store.accessToken).toBeNull()
    expect(store.restoring).toBe(false)
    expect(mockRefreshApi).not.toHaveBeenCalled()
  })

  it('tryRestoreSession calls refreshApi and sets auth on success', async () => {
    const store = new AuthStore()
    const user = makeUser()
    const { refreshToken } = makeTokens()
    const newAccess = faker.string.alphanumeric(32)
    const newRefresh = faker.string.alphanumeric(48)

    localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
    mockRefreshApi.mockResolvedValue({ accessToken: newAccess, refreshToken: newRefresh, user })

    await store.tryRestoreSession()

    expect(mockRefreshApi).toHaveBeenCalledWith({ refreshToken })
    expect(store.accessToken).toBe(newAccess)
    expect(store.user).toEqual(user)
    expect(localStorage.getItem(REFRESH_TOKEN_KEY)).toBe(newRefresh)
    expect(store.restoring).toBe(false)
  })

  it('tryRestoreSession calls clearAuth when refresh fails', async () => {
    const store = new AuthStore()
    const user = makeUser()
    const { accessToken, refreshToken } = makeTokens()

    store.setAuth(accessToken, refreshToken, user)
    localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken)
    mockRefreshApi.mockRejectedValue(new Error('Refresh failed: 401'))

    await store.tryRestoreSession()

    expect(store.accessToken).toBeNull()
    expect(store.user).toBeNull()
    expect(store.restoring).toBe(false)
  })
})
