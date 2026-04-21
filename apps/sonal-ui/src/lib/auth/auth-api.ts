export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  accessToken: string
  refreshToken: string
  user: AuthUser
}

export interface RefreshRequest {
  refreshToken: string
}

export interface RefreshResponse {
  accessToken: string
  refreshToken: string
  user: AuthUser
}

export interface MeResponse {
  user: AuthUser
}

export interface AuthUser {
  id: string
  username: string
}

const AUTH_BASE = '/api/v1/auth'

export async function loginApi(req: LoginRequest): Promise<LoginResponse> {
  const res = await fetch(`${AUTH_BASE}/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new Error(`Login failed: ${res.status}`)
  }
  return res.json() as Promise<LoginResponse>
}

export async function refreshApi(req: RefreshRequest): Promise<RefreshResponse> {
  const res = await fetch(`${AUTH_BASE}/refresh`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new Error(`Refresh failed: ${res.status}`)
  }
  return res.json() as Promise<RefreshResponse>
}

export async function meApi(accessToken: string): Promise<MeResponse> {
  const res = await fetch(`${AUTH_BASE}/me`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(`Me failed: ${res.status}`)
  }
  return res.json() as Promise<MeResponse>
}
