<script lang="ts">
  import { link } from 'svelte-spa-router'
  import SquarePen from '@lucide/svelte/icons/square-pen'
  import type { SessionMetadata } from '../lib/agentapi/types'

  let {
    sessions = [],
    activeSessionId = null,
    onNewChat,
  }: {
    sessions: SessionMetadata[]
    activeSessionId?: string | null
    onNewChat: () => void
  } = $props()

  const rtf = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })

  /** Relative label for `updatedAt` (e.g. "2 hours ago"). */
  function formatUpdatedLabel(iso: string, now = new Date()): string {
    const target = new Date(iso)
    const diffSec = Math.round((target.getTime() - now.getTime()) / 1000)
    if (Math.abs(diffSec) < 60) {
      return rtf.format(diffSec, 'second')
    }
    const diffMin = Math.round(diffSec / 60)
    if (Math.abs(diffMin) < 60) {
      return rtf.format(diffMin, 'minute')
    }
    const diffHr = Math.round(diffSec / 3600)
    if (Math.abs(diffHr) < 24) {
      return rtf.format(diffHr, 'hour')
    }
    const diffDay = Math.round(diffSec / 86400)
    if (Math.abs(diffDay) < 7) {
      return rtf.format(diffDay, 'day')
    }
    const diffWeek = Math.round(diffSec / (86400 * 7))
    if (Math.abs(diffWeek) < 5) {
      return rtf.format(diffWeek, 'week')
    }
    const diffMonth = Math.round(diffSec / (86400 * 30))
    if (Math.abs(diffMonth) < 12) {
      return rtf.format(diffMonth, 'month')
    }
    const diffYear = Math.round(diffSec / (86400 * 365))
    return rtf.format(diffYear, 'year')
  }
</script>

<div class="session-list">
  <button type="button" class="secondary new-chat" onclick={onNewChat}>
    <SquarePen size={16} strokeWidth={1.5} aria-hidden="true" />
    New chat
  </button>

  <nav class="session-nav" aria-label="Sessions">
    <ul class="session-items">
      {#each sessions as s (s.sessionId)}
        <li>
          <a
            class="session-link"
            class:active={activeSessionId !== null && s.sessionId === activeSessionId}
            href="/chat/{encodeURIComponent(s.sessionId)}"
            use:link
            aria-current={activeSessionId !== null && s.sessionId === activeSessionId
              ? 'page'
              : undefined}
          >
            <span class="session-title">{s.title}</span>
            <time class="session-time" datetime={s.updatedAt}>{formatUpdatedLabel(s.updatedAt)}</time>
          </a>
        </li>
      {/each}
    </ul>
  </nav>
</div>

<style>
  .session-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-12);
    min-height: 0;
    font-size: var(--font-size-body);
    font-weight: 400;
    line-height: 1.5;
  }

  .new-chat {
    align-self: stretch;
    flex-shrink: 0;
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: center;
    gap: var(--space-8);
    width: 100%;
    box-sizing: border-box;
  }

  .session-nav {
    min-height: 0;
    flex: 1;
    display: flex;
    flex-direction: column;
  }

  .session-items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    overflow-y: auto;
    flex: 1;
    min-height: 0;
    padding-right: var(--space-4);
  }

  .session-link {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    align-items: flex-start;
    padding: var(--space-8) var(--space-12);
    border-radius: var(--radius-default);
    border: 1px solid var(--border);
    background: transparent;
    color: var(--text);
    text-decoration: none;
    font-weight: 500;
  }

  .session-link:hover {
    border-color: var(--color-border-outline);
  }

  .session-link:focus-visible {
    outline: 1px solid var(--color-accent-blue);
    outline-offset: 0;
  }

  .session-link.active {
    background: var(--surface-raised);
    border-color: var(--color-border-outline);
  }

  .session-title {
    font-size: var(--font-size-body);
    font-weight: 500;
    line-height: 1.25;
    word-break: break-word;
  }

  .session-time {
    font-size: var(--font-size-caption);
    font-weight: 400;
    line-height: 2;
    color: var(--text-muted);
  }

</style>
