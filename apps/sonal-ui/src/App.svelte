<script lang="ts">
  import { onMount } from 'svelte'
  import Router, { replace, router } from 'svelte-spa-router'
  import { wrap } from 'svelte-spa-router/wrap'
  import Nav from './components/Nav.svelte'
  import { themeStore } from './lib/theme/theme-store.svelte'
  import Chat from './pages/Chat.svelte'
  import Login from './pages/Login.svelte'
  import Providers from './pages/Providers.svelte'
  import RedirectToChat from './pages/RedirectToChat.svelte'
  import { authStore } from './lib/auth/auth-store.svelte'

  const routes = {
    '/login': Login,
    '/': RedirectToChat,
    '/chat/:sessionId?': wrap({
      component: Chat,
      conditions: [() => authStore.isAuthenticated],
    }),
    '/providers': wrap({
      component: Providers,
      conditions: [() => authStore.isAuthenticated],
    }),
  }

  function handleConditionsFailed() {
    replace('/login')
  }

  onMount(async () => {
    await authStore.tryRestoreSession()
  })

  /** Full-viewport chat layout: sidebar flush left, no document scroll; see `.main--chat` / `body` class. */
  const isChatRoute = $derived(
    typeof router.location === 'string' && router.location.startsWith('/chat'),
  )

  $effect(() => {
    if (typeof document === 'undefined') return
    document.body.classList.toggle('chat-route-fullheight', isChatRoute)
  })
</script>

{#if authStore.restoring}
  <div class="shell loading" aria-busy="true" aria-label="Loading">
    <span class="loading-indicator">Loading…</span>
  </div>
{:else}
  <div class="shell" class:shell--chat={isChatRoute}>
    <span class="sr-only" aria-hidden="true">{themeStore.preference}</span>
    {#if authStore.isAuthenticated}
      <Nav />
    {/if}
    <main class="main" class:main--chat={isChatRoute}>
      <!-- Inner column: DESIGN.md ~800–900px reading width; `/chat` uses full width (see `.main-inner--chat`). -->
      <div class="main-inner" class:main-inner--chat={isChatRoute}>
        <Router {routes} onConditionsFailed={handleConditionsFailed} />
      </div>
    </main>
  </div>
{/if}

<style>
  .shell {
    min-height: 100vh;
    min-height: 100dvh;
    display: flex;
    flex-direction: column;
    background: var(--bg);
    color: var(--text);
  }

  /**
   * Chat: one screen below Nav — no page-level scroll; transcript scrolls inside Chat.
   */
  .shell--chat {
    height: 100dvh;
    max-height: 100dvh;
    min-height: 100dvh;
  }

  .main {
    flex: 1;
    display: flex;
    flex-direction: column;
    padding: var(--main-padding-block) var(--main-padding-inline);
    box-sizing: border-box;
    min-height: 0;
  }

  .main--chat {
    padding: 0;
    min-height: 0;
  }

  .main-inner {
    width: 100%;
    max-width: var(--content-max-width);
    margin-inline: auto;
    box-sizing: border-box;
    flex: 1;
    min-height: 0;
    display: flex;
    flex-direction: column;
  }

  .main-inner--chat {
    max-width: none;
    margin-inline: 0;
    flex: 1;
    min-height: 0;
  }

  .loading {
    align-items: center;
    justify-content: center;
    background: var(--bg);
    color: var(--text-h);
  }

  .loading-indicator {
    font-size: var(--font-size-body);
    font-weight: 500;
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

  :global(body.chat-route-fullheight) {
    overflow: hidden;
  }
</style>
