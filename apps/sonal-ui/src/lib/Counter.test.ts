import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import Counter from './Counter.svelte'

describe('Counter', () => {
  it('increments label on click', async () => {
    render(Counter)
    const user = userEvent.setup()
    const btn = screen.getByRole('button', { name: /count is 0/i })
    await user.click(btn)
    expect(screen.getByRole('button', { name: /count is 1/i })).toBeInTheDocument()
  })
})
