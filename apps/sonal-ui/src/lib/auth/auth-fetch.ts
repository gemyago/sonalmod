import { refreshApi } from './auth-api'
import type { AuthStore } from './auth-store.svelte'

/**
 * Returns a fetch wrapper that:
 * 1. Injects `Authorization: Bearer <accessToken>` if a token is present.
 * 2. On 401, attempts a token refresh and retries the original request once.
 * 3. On refresh failure, clears auth state and returns the 401 response.
 */
export function createAuthFetch(
  authStore: AuthStore,
): (input: RequestInfo | URL, init?: RequestInit) => Promise<Response> {
  return async function authFetch(
    input: RequestInfo | URL,
    init?: RequestInit,
  ): Promise<Response> {
    const res = await fetch(input, withAuth(init, authStore.accessToken))

    if (res.status !== 401) {
      return res
    }

    // Attempt token refresh
    try {
      const refreshToken = localStorage.getItem('auth_refresh_token')
      if (!refreshToken) {
        authStore.clearAuth()
        return res
      }
      const refreshed = await refreshApi({ refreshToken })
      authStore.setAuth(refreshed.accessToken, refreshed.refreshToken, refreshed.user)
    } catch {
      authStore.clearAuth()
      return res
    }

    // Retry with new token
    return fetch(input, withAuth(init, authStore.accessToken))
  }
}

function withAuth(init: RequestInit | undefined, accessToken: string | null): RequestInit {
  if (!accessToken) {
    return init ?? {}
  }
  const existing = (init?.headers as Record<string, string> | undefined) ?? {}
  return {
    ...init,
    headers: {
      ...existing,
      Authorization: `Bearer ${accessToken}`,
    },
  }
}
