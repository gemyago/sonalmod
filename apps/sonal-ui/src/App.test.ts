import { describe, it, expect, beforeEach, vi } from 'vitest'
import { faker } from '@faker-js/faker'
import { render, screen } from '@testing-library/svelte'
import { waitFor } from '@testing-library/dom'
import userEvent from '@testing-library/user-event'
import App from './App.svelte'

// Mock the auth store so tests can control authentication state
const mocks = vi.hoisted(() => ({
  isAuthenticated: true,
  restoring: false,
  tryRestoreSession: vi.fn().mockResolvedValue(undefined),
  clearAuth: vi.fn(),
  accessToken: null as string | null,
  user: null as { id: string; username: string } | null,
}))

vi.mock('./lib/auth/auth-store.svelte', () => ({
  authStore: mocks,
}))

describe('App shell', () => {
  beforeEach(() => {
    window.location.hash = '#/chat'
    mocks.isAuthenticated = true
    mocks.restoring = false
    mocks.tryRestoreSession.mockResolvedValue(undefined)
    mocks.accessToken = faker.string.alphanumeric(32)
  })

  it('shows Chat heading on initial load when authenticated', async () => {
    render(App)
    expect(
      await screen.findByRole('heading', { name: 'Chat' }),
    ).toBeInTheDocument()
  })

  it('navigates to Providers when Providers link is clicked', async () => {
    const user = userEvent.setup()
    render(App)
    await user.click(screen.getByRole('link', { name: 'Providers' }))
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
    })
  })

  it('shows the message composer when the hash includes a session id', async () => {
    const sessionSlug = faker.string.uuid()
    window.location.hash = `#/chat/${sessionSlug}`
    render(App)
    expect(
      await screen.findByRole('textbox', { name: 'Message' }),
    ).toBeInTheDocument()
  })

  it('redirects to /login when navigating to protected route unauthenticated', async () => {
    mocks.isAuthenticated = false
    render(App)
    await waitFor(() => {
      expect(window.location.hash).toBe('#/login')
    })
  })

  it('shows loading indicator while session is restoring', async () => {
    mocks.restoring = true
    render(App)
    expect(screen.getByLabelText('Loading')).toBeInTheDocument()
  })
})
