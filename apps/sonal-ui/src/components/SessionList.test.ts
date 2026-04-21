import { describe, it, expect, vi } from 'vitest'
import { faker } from '@faker-js/faker'
import { render, screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import SessionList from './SessionList.svelte'
import type { SessionMetadata } from '../lib/agentapi/types'

function linkNameMatcher(title: string): RegExp {
  return new RegExp(title.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
}

function sessionFixture(overrides: Partial<SessionMetadata> & Pick<SessionMetadata, 'sessionId' | 'title'>): SessionMetadata {
  const created = faker.date.past()
  return {
    createdAt: created.toISOString(),
    updatedAt: faker.date.between({ from: created, to: new Date() }).toISOString(),
    ...overrides,
  }
}

describe('SessionList', () => {
  it('renders session entries with titles', () => {
    const titleA = faker.lorem.words(3)
    const titleB = faker.lorem.words(3)
    const sessions: SessionMetadata[] = [
      sessionFixture({ sessionId: faker.string.uuid(), title: titleA }),
      sessionFixture({ sessionId: faker.string.uuid(), title: titleB }),
    ]
    const onNewChat = vi.fn()
    render(SessionList, { props: { sessions, activeSessionId: null, onNewChat } })

    expect(screen.getByText(titleA)).toBeInTheDocument()
    expect(screen.getByText(titleB)).toBeInTheDocument()
  })

  it('highlights active session', () => {
    const activeId = faker.string.uuid()
    const titleActive = faker.lorem.words(2)
    const titleOther = faker.lorem.words(2)
    const sessions: SessionMetadata[] = [
      sessionFixture({ sessionId: activeId, title: titleActive }),
      sessionFixture({ sessionId: faker.string.uuid(), title: titleOther }),
    ]
    const onNewChat = vi.fn()
    render(SessionList, {
      props: { sessions, activeSessionId: activeId, onNewChat },
    })

    const current = screen.getByRole('link', { name: linkNameMatcher(titleActive) })
    const other = screen.getByRole('link', { name: linkNameMatcher(titleOther) })
    expect(current).toHaveAttribute('aria-current', 'page')
    expect(other).not.toHaveAttribute('aria-current')
  })

  it('calls onNewChat when New chat is clicked', async () => {
    const onNewChat = vi.fn()
    render(SessionList, { props: { sessions: [], activeSessionId: null, onNewChat } })

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'New chat' }))

    expect(onNewChat).toHaveBeenCalledTimes(1)
  })
})
