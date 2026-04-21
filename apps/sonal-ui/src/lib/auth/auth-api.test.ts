import { describe, it, expect, beforeAll, afterEach, afterAll } from 'vitest'
import { faker } from '@faker-js/faker'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { loginApi, refreshApi, meApi } from './auth-api'

function makeUser() {
  return { id: faker.string.uuid(), username: faker.internet.username() }
}

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

describe('loginApi', () => {
  it('sends POST /api/v1/auth/login with credentials and returns tokens + user', async () => {
    const user = makeUser()
    const accessToken = faker.string.alphanumeric(32)
    const refreshToken = faker.string.alphanumeric(48)
    let capturedBody: unknown
    server.use(
      http.post('/api/v1/auth/login', async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json({ accessToken, refreshToken, user })
      }),
    )

    const req = { username: user.username, password: faker.internet.password() }
    const res = await loginApi(req)

    expect(capturedBody).toEqual(req)
    expect(res.accessToken).toBe(accessToken)
    expect(res.refreshToken).toBe(refreshToken)
    expect(res.user).toEqual(user)
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.post('/api/v1/auth/login', () => HttpResponse.json({}, { status: 401 })),
    )
    await expect(loginApi({ username: 'x', password: 'y' })).rejects.toThrow('Login failed: 401')
  })
})

describe('refreshApi', () => {
  it('sends POST /api/v1/auth/refresh with refreshToken and returns new tokens + user', async () => {
    const user = makeUser()
    const newAccess = faker.string.alphanumeric(32)
    const newRefresh = faker.string.alphanumeric(48)
    const oldRefresh = faker.string.alphanumeric(48)
    let capturedBody: unknown
    server.use(
      http.post('/api/v1/auth/refresh', async ({ request }) => {
        capturedBody = await request.json()
        return HttpResponse.json({ accessToken: newAccess, refreshToken: newRefresh, user })
      }),
    )

    const res = await refreshApi({ refreshToken: oldRefresh })

    expect(capturedBody).toEqual({ refreshToken: oldRefresh })
    expect(res.accessToken).toBe(newAccess)
    expect(res.refreshToken).toBe(newRefresh)
    expect(res.user).toEqual(user)
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.post('/api/v1/auth/refresh', () => HttpResponse.json({}, { status: 401 })),
    )
    await expect(refreshApi({ refreshToken: 'bad' })).rejects.toThrow('Refresh failed: 401')
  })
})

describe('meApi', () => {
  it('sends GET /api/v1/auth/me with Bearer token and returns user', async () => {
    const user = makeUser()
    const token = faker.string.alphanumeric(32)
    let capturedAuthHeader: string | null = null
    server.use(
      http.get('/api/v1/auth/me', ({ request }) => {
        capturedAuthHeader = request.headers.get('Authorization')
        return HttpResponse.json({ user })
      }),
    )

    const res = await meApi(token)

    expect(capturedAuthHeader).toBe(`Bearer ${token}`)
    expect(res.user).toEqual(user)
  })

  it('throws on non-ok response', async () => {
    server.use(
      http.get('/api/v1/auth/me', () => HttpResponse.json({}, { status: 401 })),
    )
    await expect(meApi('bad')).rejects.toThrow('Me failed: 401')
  })
})
