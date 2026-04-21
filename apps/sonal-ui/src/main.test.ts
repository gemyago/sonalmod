import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

describe('main entry', () => {
  beforeEach(() => {
    document.body.innerHTML = '<div id="app"></div>'
  })

  afterEach(() => {
    vi.resetModules()
  })

  it('mounts the app shell into #app', async () => {
    await import('./main')
    const root = document.getElementById('app')
    expect(root?.childElementCount).toBeGreaterThan(0)
  })
})
