<script lang="ts">
  import { onMount, untrack, tick } from 'svelte'
  import { replace, push, link } from 'svelte-spa-router'
  import { createSonalAgentApi } from '../lib/agentapi/client'
  import { authStore } from '../lib/auth/auth-store.svelte'
  import { parseAgentSseJsonStream } from '../lib/agentapi/sse'
  import {
    applyAgentStreamEvent,
    applyIdleHistoryAgentEvent,
    createInitialAgentRunStreamState,
    type AgentRunStreamState,
    type ChatTranscriptMessage,
  } from '../lib/agentapi/streamState'
  import {
    isStreamEvent,
    type AgentRunRequest,
    type ModelInfo,
    type SessionMetadata,
  } from '../lib/agentapi/types'
  import Send from '@lucide/svelte/icons/send'
  import SessionList from '../components/SessionList.svelte'
  import ToolCallBlock from '../components/ToolCallBlock.svelte'

  /**
   * Agent API origin or path prefix. Default matches sonalmod mount (`/api/v1/runtime/`, port 4501)
   * and Vite dev `server.proxy` so dev works without a `.env` copy.
   */
  const agentBaseUrl = import.meta.env.VITE_AGENT_API_BASE_URL ?? '/api/v1/runtime'

  const agentApi = $derived.by(() =>
    createSonalAgentApi({ baseUrl: agentBaseUrl, accessToken: authStore.accessToken }),
  )

  const MODEL_STORAGE_KEY = 'selectedModel'

  let { params = {} } = $props<{ params?: { sessionId?: string | null } }>()

  let messages = $state<ChatTranscriptMessage[]>([])
  let inputText = $state('')
  let sendDisabled = $state(false)
  let runError = $state<string | null>(null)
  let availableModels = $state<ModelInfo[]>([])
  let selectedModel = $state<string>(localStorage.getItem(MODEL_STORAGE_KEY) ?? '')
  /** `loading` until the first `listModels` attempt finishes. */
  let modelsLoadStatus = $state<'loading' | 'success' | 'empty' | 'error'>('loading')
  let modelsListErrorMessage = $state<string | null>(null)

  let sessionList = $state<SessionMetadata[]>([])
  /** Narrow viewport: sidebar starts collapsed; toggle opens it. */
  let sidebarCollapsed = $state(false)
  let isNarrowViewport = $state(false)
  /** Transcript strip; scroll only when content exceeds max-height. */
  let messagesScrollEl = $state<HTMLDivElement | undefined>(undefined)

  onMount(() => {
    if (typeof window.matchMedia !== 'function') {
      return
    }
    const mq = window.matchMedia('(max-width: 768px)')
    const apply = () => {
      isNarrowViewport = mq.matches
      if (mq.matches) {
        sidebarCollapsed = true
      } else {
        sidebarCollapsed = false
      }
    }
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  })

  async function refreshSessions() {
    try {
      const res = await agentApi.listSessions({ limit: 50 })
      sessionList = res.sessions
    } catch {
      // Best-effort; listing is secondary to chat.
    }
  }

  $effect(() => {
    void agentApi
    void refreshSessions()
  })

  $effect(() => {
    const api = agentApi
    let cancelled = false
    modelsLoadStatus = 'loading'
    modelsListErrorMessage = null
    api
      .listModels()
      .then((res) => {
        if (cancelled) {
          return
        }
        availableModels = res.models
        if (res.models.length === 0) {
          modelsLoadStatus = 'empty'
          selectedModel = ''
          return
        }
        modelsLoadStatus = 'success'
        const stored = localStorage.getItem(MODEL_STORAGE_KEY)
        const valid = res.models.find((m) => `${m.provider}/${m.name}` === stored)
        const pick = valid ?? res.models[0]
        const id = `${pick.provider}/${pick.name}`
        selectedModel = id
        localStorage.setItem(MODEL_STORAGE_KEY, id)
      })
      .catch((err) => {
        if (cancelled) {
          return
        }
        modelsLoadStatus = 'error'
        modelsListErrorMessage = err instanceof Error ? err.message : 'Failed to load models'
        availableModels = []
      })
    return () => {
      cancelled = true
    }
  })

  const canSendMessage = $derived.by(() => {
    if (modelsLoadStatus !== 'success' || availableModels.length === 0) {
      return false
    }
    if (!selectedModel) {
      return false
    }
    return availableModels.some((m) => `${m.provider}/${m.name}` === selectedModel)
  })

  let streamState = $state<AgentRunStreamState>(
    createInitialAgentRunStreamState({ busy: false }),
  )

  /** URL-driven `readSession` only — must not share a signal with {@link runAbortController}. */
  let hydrationAbortController: AbortController | null = null
  /** `startAgentRun` / `continueAgentRun` — must not share a signal with {@link hydrationAbortController}. */
  let runAbortController: AbortController | null = null
  /**
   * While an agent run POST owns this session (including between `sessionBound` and stream `done`),
   * do not run URL-driven `readSession` — the send response stream is the source of truth.
   */
  let runOwningSessionId = $state<string | null>(null)

  $effect(() => {
    const sessionId = params.sessionId
    if (!sessionId) {
      return
    }
    // Do not subscribe to runOwningSessionId here: clearing it in `finally` after a send must not
    // re-run this effect and start readSession again.
    const owning = untrack(() => runOwningSessionId)
    if (owning !== null && owning === sessionId) {
      return
    }
    hydrationAbortController?.abort()
    hydrationAbortController = new AbortController()
    const signal = hydrationAbortController.signal
    loadSession(sessionId, signal)
  })

  $effect(() => {
    return () => {
      hydrationAbortController?.abort()
      runAbortController?.abort()
    }
  })

  /** Keep the latest turn in view as the transcript grows (internal scroll only when capped). */
  $effect(() => {
    void messages
    void streamState
    void runError
    void messagesScrollEl
    const el = messagesScrollEl
    if (!el) {
      return
    }
    void tick().then(() => {
      el.scrollTop = el.scrollHeight
    })
  })

  async function loadSession(sessionId: string, signal: AbortSignal) {
    runError = null
    sendDisabled = true
    messages = []
    // Start with busy=true so we show "Thinking…" while we don't yet know if active/idle.
    streamState = createInitialAgentRunStreamState({ sessionId, busy: true })

    /** Idle-only: scratch assistant state + rows for historical `agent` events (not merged into main streamState). */
    let replayAssistState = createInitialAgentRunStreamState({ sessionId, busy: false })
    let idleReplayRows: ChatTranscriptMessage[] = []
    // Whether the session has an active run (set by sessionStatus event).
    let isActive = false

    try {
      const response = await agentApi.readSession({ sessionId, signal })
      if (!response.ok) {
        runError = `Failed to load session (${response.status})`
        streamState = createInitialAgentRunStreamState({ sessionId, busy: false })
        return
      }
      if (!response.body) {
        runError = 'No response body'
        streamState = createInitialAgentRunStreamState({ sessionId, busy: false })
        return
      }

      for await (const ev of parseAgentSseJsonStream(response.body, { signal })) {
        if (!isStreamEvent(ev.payload)) {
          continue
        }
        const event = ev.payload

        if (event.event === 'agent' && !isActive) {
          const { state: nextReplay, appended } = applyIdleHistoryAgentEvent(replayAssistState, event)
          replayAssistState = nextReplay
          idleReplayRows = [...idleReplayRows, ...appended]
        } else {
          streamState = applyAgentStreamEvent(streamState, event)
        }

        if (event.event === 'sessionStatus') {
          isActive = event.status === 'active'
          if (!isActive) {
            // Idle: reset busy so the composer shows ready after history loads.
            streamState = { ...streamState, busy: false }
          }
        }

        if (event.event === 'done') {
          if (isActive) {
            // Live run finished: commit the streamed assistant turn to messages.
            const assistantText = streamState.assistantTurnText
            const toolCalls = streamState.toolCalls
            if (assistantText || toolCalls.length > 0) {
              const row: ChatTranscriptMessage = { role: 'assistant', text: assistantText }
              if (toolCalls.length > 0) {
                row.toolCalls = [...toolCalls]
              }
              messages = [...messages, row]
            }
          } else {
            const tailText = replayAssistState.assistantTurnText
            const tailToolCalls = replayAssistState.toolCalls
            const tail: ChatTranscriptMessage[] =
              tailText || tailToolCalls.length > 0
                ? [
                    {
                      role: 'assistant',
                      text: tailText,
                      ...(tailToolCalls.length > 0 ? { toolCalls: [...tailToolCalls] } : {}),
                    },
                  ]
                : []
            messages = [...messages, ...idleReplayRows, ...tail]
          }
          streamState = createInitialAgentRunStreamState({ sessionId, busy: false })
        }

        if (event.event === 'error') {
          runError = streamState.error
        }
      }
    } catch (err) {
      if (signal.aborted) {
        return
      }
      runError = err instanceof Error ? err.message : 'Failed to load session'
      streamState = createInitialAgentRunStreamState({ sessionId, busy: false })
    } finally {
      sendDisabled = false
    }
  }

  function effectiveSessionId(): string | null {
    return params.sessionId ?? streamState.sessionId
  }

  function handleNewChat() {
    runOwningSessionId = null
    hydrationAbortController?.abort()
    runAbortController?.abort()
    push('/chat')
    messages = []
    inputText = ''
    runError = null
    streamState = createInitialAgentRunStreamState({ busy: false })
  }

  async function commitSend() {
    const text = inputText.trim()
    if (!text || sendDisabled || !canSendMessage) {
      return
    }

    hydrationAbortController?.abort()
    runAbortController?.abort()
    runAbortController = new AbortController()
    const signal = runAbortController.signal

    const sessionId = effectiveSessionId()
    runOwningSessionId = sessionId
    const body: AgentRunRequest = {
      model: selectedModel,
      message: { parts: [{ text }] },
    }

    messages = [...messages, { role: 'user', text }]
    inputText = ''
    runError = null
    sendDisabled = true

    streamState = createInitialAgentRunStreamState({
      sessionId,
      busy: true,
      assistantTurnText: '',
      error: null,
    })

    try {
      const response = sessionId
        ? await agentApi.continueAgentRun({ sessionId, body, signal })
        : await agentApi.startAgentRun({ body, signal })

      if (!response.ok) {
        runError = `Request failed (${response.status})`
        streamState = createInitialAgentRunStreamState({
          sessionId,
          busy: false,
          assistantTurnText: '',
          error: null,
        })
        return
      }
      if (!response.body) {
        runError = 'No response body'
        streamState = createInitialAgentRunStreamState({
          sessionId,
          busy: false,
          assistantTurnText: '',
          error: null,
        })
        return
      }

      for await (const ev of parseAgentSseJsonStream(response.body, { signal })) {
        if (!isStreamEvent(ev.payload)) {
          continue
        }
        const event = ev.payload
        streamState = applyAgentStreamEvent(streamState, event)

        if (event.event === 'sessionBound') {
          runOwningSessionId = event.sessionId
          replace(`/chat/${encodeURIComponent(event.sessionId)}`)
        }

        if (event.event === 'done') {
          const assistantText = streamState.assistantTurnText
          const toolCalls = streamState.toolCalls
          const row: ChatTranscriptMessage = { role: 'assistant', text: assistantText }
          if (toolCalls.length > 0) {
            row.toolCalls = [...toolCalls]
          }
          messages = [...messages, row]
          streamState = createInitialAgentRunStreamState({
            sessionId: streamState.sessionId,
            busy: false,
            assistantTurnText: '',
            error: null,
          })
          void refreshSessions()
        }

        if (event.event === 'error') {
          runError = streamState.error
        }
      }
    } catch (err) {
      if (signal.aborted) {
        return
      }
      runError = err instanceof Error ? err.message : 'Request failed'
      streamState = createInitialAgentRunStreamState({
        sessionId: effectiveSessionId(),
        busy: false,
        assistantTurnText: '',
        error: null,
      })
    } finally {
      sendDisabled = false
      runOwningSessionId = null
    }
  }

  function handleSubmit(e: Event) {
    e.preventDefault()
    void commitSend()
  }

  function handleComposerKeydown(e: KeyboardEvent) {
    if (e.key !== 'Enter' || e.shiftKey) {
      return
    }
    if (e.isComposing) {
      return
    }
    e.preventDefault()
    void commitSend()
  }
</script>

<section class="page chat-page" aria-labelledby="chat-heading">
  <div
    class="chat-page-shell"
    class:sidebar-collapsed={isNarrowViewport && sidebarCollapsed}
  >
    <aside class="chat-sidebar" aria-label="Session list">
      <SessionList
        sessions={sessionList}
        activeSessionId={params.sessionId ?? null}
        onNewChat={handleNewChat}
      />
    </aside>

    <div class="chat-main">
      <header class="header">
        <div class="header-row">
          <div class="header-copy">
            <h1 id="chat-heading">Chat</h1>
          </div>
          {#if isNarrowViewport}
            <button
              type="button"
              class="secondary sidebar-toggle-main"
              aria-expanded={!sidebarCollapsed}
              onclick={() => (sidebarCollapsed = !sidebarCollapsed)}
            >
              Sessions
            </button>
          {/if}
        </div>
      </header>

    <div class="chat-layout">
      <div
        class="messages-scroll"
        bind:this={messagesScrollEl}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        aria-label="Chat transcript and model activity"
      >
        <ul class="messages">
          {#each messages as m, i (i)}
            <li class="message-group">
              {#if m.text}
                <div class="bubble" class:user={m.role === 'user'} class:assistant={m.role === 'assistant'}><span class="sr-only">{m.role === 'user' ? 'You' : 'Assistant'}:</span>{m.text}</div>
              {/if}
              {#if m.toolCalls?.length}
                {#each m.toolCalls as tc (tc.id)}
                  <ToolCallBlock name={tc.name} args={tc.args} response={tc.response} />
                {/each}
              {/if}
            </li>
          {/each}
        </ul>

        {#if streamState.busy && streamState.assistantTurnText}
          <div class="turn-activity bubble assistant streaming" aria-busy="true"><span class="sr-only">Assistant:</span>{streamState.assistantTurnText}</div>
        {:else if streamState.busy}
          <div class="turn-activity bubble assistant streaming muted" aria-busy="true"><span class="sr-only">Assistant:</span>Thinking…</div>
        {/if}

        {#if streamState.toolCalls.length > 0}
          <div class="turn-tool-calls">
            {#each streamState.toolCalls as tc (tc.id)}
              <ToolCallBlock name={tc.name} args={tc.args} response={tc.response} />
            {/each}
          </div>
        {/if}

        {#if runError}
          <p class="error turn-error" role="alert">{runError}</p>
        {/if}
      </div>

      {#if modelsLoadStatus === 'loading'}
        <p class="model-banner muted">Loading models…</p>
      {:else if modelsLoadStatus === 'error'}
        <p class="error model-banner" role="alert">{modelsListErrorMessage ?? 'Failed to load models'}</p>
      {:else if modelsLoadStatus === 'empty'}
        <p class="model-banner model-empty" role="status">
          No models available. Add models under
          <a href="/providers" use:link>Providers</a>.
        </p>
      {/if}

      <form class="composer" onsubmit={handleSubmit}>
        <label class="sr-only" for="chat-input">Message</label>
        <textarea
          id="chat-input"
          class="input"
          rows="3"
          bind:value={inputText}
          placeholder="Type a message…"
          disabled={sendDisabled}
          onkeydown={handleComposerKeydown}
        ></textarea>
        <div class="composer-bar">
          <div class="composer-bar-start">
            {#if modelsLoadStatus === 'success' && availableModels.length > 0}
              <div class="model-picker">
                <label class="sr-only" for="model-select">Model</label>
                <select
                  id="model-select"
                  class="model-select"
                  bind:value={selectedModel}
                  onchange={() => localStorage.setItem(MODEL_STORAGE_KEY, selectedModel)}
                  disabled={sendDisabled}
                >
                  {#each availableModels as m (`${m.provider}/${m.name}`)}
                    <option value={`${m.provider}/${m.name}`}>{m.displayName ?? m.name}</option>
                  {/each}
                </select>
              </div>
            {/if}
          </div>
          <button
            type="submit"
            class="primary composer-send"
            disabled={sendDisabled || !inputText.trim() || !canSendMessage}
          >
            <Send size={18} strokeWidth={1.5} aria-hidden="true" />
            Send
          </button>
        </div>
      </form>
    </div>
    </div>
  </div>
</section>

<style>
  .page {
    width: 100%;
    max-width: none;
    text-align: left;
  }

  .chat-page {
    flex: 1;
    min-height: 0;
    width: 100%;
    display: flex;
    flex-direction: column;
  }

  .chat-page-shell {
    display: flex;
    flex-direction: row;
    align-items: stretch;
    gap: var(--space-16);
    position: relative;
    flex: 1;
    min-height: 0;
  }

  .chat-sidebar {
    width: 200px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    min-height: 0;
    align-self: stretch;
    padding-top: var(--space-16);
    padding-left: var(--main-padding-inline);
    padding-right: var(--space-12);
    border-right: 1px solid var(--border);
  }

  .chat-main {
    flex: 1;
    min-width: 0;
    min-height: 0;
    display: flex;
    flex-direction: column;
    padding-top: var(--space-16);
    padding-right: var(--main-padding-inline);
    padding-bottom: var(--space-16);
    box-sizing: border-box;
  }

  .header-row {
    display: flex;
    flex-direction: row;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-12);
  }

  .header-copy {
    min-width: 0;
  }

  .sidebar-toggle-main {
    flex-shrink: 0;
  }

  @media (max-width: 768px) {
    .chat-page-shell {
      flex-direction: column;
      gap: var(--space-12);
    }

    .chat-sidebar {
      position: fixed;
      top: 0;
      left: 0;
      bottom: 0;
      z-index: 20;
      width: min(200px, 72vw);
      padding: var(--space-16);
      padding-right: var(--space-12);
      background: var(--surface);
      border-right: 1px solid var(--border);
      box-shadow: 4px 0 24px rgba(0, 0, 0, 0.12);
      transform: translateX(0);
      transition: transform 0.2s ease;
    }

    :global(.dark) .chat-sidebar {
      box-shadow: 4px 0 24px rgba(0, 0, 0, 0.35);
    }

    .chat-page-shell.sidebar-collapsed .chat-sidebar {
      transform: translateX(-100%);
      pointer-events: none;
    }
  }

  .header {
    margin-bottom: var(--space-16);
  }

  h1 {
    font-size: var(--font-size-h1);
    font-weight: 700;
    color: var(--text-h);
    margin: 0;
  }

  .chat-layout {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
  }

  /** Fills space between header and composer; scrolls internally only. */
  .messages-scroll {
    flex: 1 1 0;
    min-height: 0;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    margin-bottom: var(--space-16);
    padding-right: var(--space-4);
  }

  .messages {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    flex-shrink: 0;
  }

  .message-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
  }

  .turn-tool-calls {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    flex-shrink: 0;
  }

  .turn-activity {
    flex-shrink: 0;
  }

  .turn-error {
    flex-shrink: 0;
    margin-top: 0;
  }

  .bubble {
    margin: 0;
    padding: var(--space-12) var(--space-16);
    border-radius: var(--radius-default);
    border: 1px solid var(--border);
    font-size: var(--font-size-body);
    font-weight: 400;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .bubble.user {
    align-self: flex-end;
    background: var(--surface-raised);
    border-color: var(--color-border-outline);
    color: var(--text-on-raised);
  }

  .bubble.assistant {
    align-self: flex-start;
    padding: var(--space-8) 0;
    border: none;
    background: transparent;
    color: var(--text);
  }

  .bubble.muted {
    color: var(--text);
    font-style: italic;
    font-weight: 400;
  }

  .model-banner {
    flex-shrink: 0;
    margin: 0 0 var(--space-8);
    font-size: var(--font-size-body);
    line-height: 1.5;
  }

  .model-banner.muted {
    color: var(--text);
    font-style: italic;
    font-weight: 400;
  }

  .model-empty a {
    font-weight: 500;
  }

  .composer {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-12);
  }

  .composer-bar {
    display: flex;
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-12);
    flex-wrap: wrap;
  }

  .composer-bar-start {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    align-items: center;
  }

  .model-picker {
    flex: 0 1 auto;
    min-width: 0;
    max-width: 100%;
    display: flex;
    align-items: center;
    gap: var(--space-8);
    font-size: var(--font-size-body);
    font-weight: 500;
    line-height: 1;
  }

  .model-select {
    font: inherit;
    font-weight: 400;
    width: auto;
    max-width: min(14rem, 100%);
    padding: var(--space-8) var(--space-12);
    border-radius: var(--radius-input);
    border: 1px solid var(--color-border-outline);
    background: var(--color-input-bg);
    color: var(--color-input-text);
    cursor: pointer;
  }

  .model-select:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }

  .composer-send {
    flex-shrink: 0;
    margin-left: auto;
    display: inline-flex;
    flex-direction: row;
    align-items: center;
    gap: var(--space-8);
  }

  .composer-send :global(svg) {
    flex-shrink: 0;
    color: currentColor;
  }

  .input {
    font: inherit;
    padding: var(--space-16);
    border-radius: var(--radius-input);
    border: 1px solid var(--color-border-outline);
    background: var(--color-input-bg);
    color: var(--color-input-text);
    resize: vertical;
    min-height: 4rem;
    box-sizing: border-box;
  }

  .input:focus-visible {
    outline: 1px solid var(--color-accent-blue);
    outline-offset: 0;
  }

  .input:disabled {
    opacity: 0.55;
  }

  .error {
    margin: 0 0 var(--space-12);
    padding: var(--space-8) var(--space-12);
    border-radius: var(--radius-default);
    background: var(--danger-bg);
    border: 1px solid var(--color-danger);
    color: var(--color-danger);
    font-size: var(--font-size-caption);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
