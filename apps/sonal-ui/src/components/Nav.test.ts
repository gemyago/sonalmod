import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Nav from './Nav.svelte'

const mocks = vi.hoisted(() => ({
  clearAuth: vi.fn(),
  replace: vi.fn(),
}))

vi.mock('../lib/auth/auth-store.svelte', () => ({
  authStore: { clearAuth: mocks.clearAuth },
}))

vi.mock('svelte-spa-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('svelte-spa-router')>()
  return {
    ...actual,
    replace: mocks.replace,
  }
})

describe('Nav', () => {
  beforeEach(() => {
    mocks.clearAuth.mockClear()
    mocks.replace.mockClear()
  })

  it('sign out clears auth and replaces route with /login', async () => {
    const user = userEvent.setup()
    render(Nav)

    await user.click(screen.getByRole('button', { name: 'Sign out' }))

    expect(mocks.clearAuth).toHaveBeenCalledTimes(1)
    expect(mocks.replace).toHaveBeenCalledWith('/login')
  })
})
