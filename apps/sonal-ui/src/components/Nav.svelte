<script lang="ts">
  import {link, replace} from 'svelte-spa-router'
  import Monitor from '@lucide/svelte/icons/monitor'
  import Moon from '@lucide/svelte/icons/moon'
  import Sun from '@lucide/svelte/icons/sun'
  import { themeStore, type ThemePreference } from '../lib/theme/theme-store.svelte'
  import { authStore } from '../lib/auth/auth-store.svelte'

  function signOut(): void {
    authStore.clearAuth()
    replace('/login')
  }

  const themeOptions: {
    value: ThemePreference
    label: string
    icon: typeof Monitor
  }[] = [
    { value: 'auto', label: 'Auto', icon: Monitor },
    { value: 'light', label: 'Light', icon: Sun },
    { value: 'dark', label: 'Dark', icon: Moon },
  ]

  function setTheme(pref: ThemePreference): void {
    themeStore.setPreference(pref)
  }

  function onThemeKeydown(e: KeyboardEvent, index: number): void {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const delta = e.key === 'ArrowRight' ? 1 : -1
    const next = (index + delta + themeOptions.length) % themeOptions.length
    const pref = themeOptions[next].value
    setTheme(pref)
    queueMicrotask(() => {
      document.getElementById(`theme-opt-${pref}`)?.focus()
    })
  }
</script>

<nav class="app-nav" aria-label="Main">
  <a class="brand" href="/chat" use:link>Sonalmod</a>
  <ul class="links">
    <li><a href="/chat" use:link>Chat</a></li>
    <li><a href="/providers" use:link>Providers</a></li>
  </ul>
  <div class="nav-end">
    <button type="button" class="sign-out" onclick={signOut}>Sign out</button>
    <div
      class="theme-seg"
      role="radiogroup"
      aria-label="Theme"
    >
      {#each themeOptions as o, i (o.value)}
        {@const Icon = o.icon}
        {@const checked = themeStore.preference === o.value}
        <button
          id="theme-opt-{o.value}"
          type="button"
          class="theme-seg__btn"
          role="radio"
          aria-checked={checked}
          aria-label={o.label}
          tabindex={checked ? 0 : -1}
          title={o.label}
          onclick={() => setTheme(o.value)}
          onkeydown={(e) => onThemeKeydown(e, i)}
        >
          <Icon size={14} strokeWidth={1.5} aria-hidden="true" />
        </button>
      {/each}
    </div>
  </div>
</nav>

<style>
  .app-nav {
    display: grid;
    grid-template-columns:
      auto
      min(var(--content-max-width), calc(100% - 2 * var(--main-padding-inline)))
      minmax(0, 1fr);
    align-items: center;
    column-gap: var(--space-16);
    padding: var(--space-16) var(--main-padding-inline);
    border-bottom: 1px solid var(--border);
    background: var(--bg);
    box-sizing: border-box;
    /* Col 1 = brand (auto so it never collapses to 0 and overlaps links). Center track = .main-inner width. */
  }

  .links {
    grid-column: 2;
    justify-self: start;
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-20);
    list-style: none;
    margin: 0;
    padding: 0;
    min-width: 0;
  }

  .links a {
    font-weight: 500;
    font-size: var(--font-size-body);
    line-height: 1;
    color: var(--link);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .links a:hover,
  .links a:focus-visible {
    color: var(--color-accent-blue);
  }

  .brand {
    grid-column: 1;
    justify-self: start;
    font-weight: 700;
    font-size: var(--font-size-body);
    line-height: 1.5;
    color: var(--link);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .brand:hover,
  .brand:focus-visible {
    color: var(--color-accent-blue);
  }

  /* Right cluster: sign out + theme — pinned to the nav’s right inset */
  .nav-end {
    grid-column: 3;
    justify-self: end;
    display: flex;
    align-items: center;
    gap: var(--space-16);
    min-width: 0;
  }

  .sign-out {
    margin: 0;
    padding: 0;
    border: none;
    font: inherit;
    font-weight: 500;
    font-size: var(--font-size-body);
    line-height: 1;
    color: var(--link);
    text-decoration: underline;
    text-underline-offset: 2px;
    background: transparent;
    cursor: pointer;
  }

  .sign-out:hover,
  .sign-out:focus-visible {
    color: var(--color-accent-blue);
  }

  .sign-out:focus-visible {
    outline: 2px solid var(--color-accent-blue);
    outline-offset: 1px;
  }

  /* Theme: compact icon-only utility */
  .theme-seg {
    display: flex;
    align-items: stretch;
    padding: 1px;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    box-sizing: border-box;
    opacity: 0.92;
  }

  .theme-seg__btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 0;
    width: 1.75rem;
    height: 1.625rem;
    padding: 0;
    border: none;
    border-radius: 999px;
    color: var(--text-muted);
    background: transparent;
    cursor: pointer;
    transition:
      background 0.1s ease,
      color 0.1s ease,
      opacity 0.1s ease;
  }

  .theme-seg__btn + .theme-seg__btn {
    box-shadow: -1px 0 0 var(--border);
  }

  .theme-seg__btn[aria-checked='true'] {
    color: var(--text-h);
    background: var(--secondary-hover-bg);
  }

  .theme-seg__btn:hover:not([aria-checked='true']) {
    color: var(--text);
  }

  .theme-seg__btn:focus-visible {
    outline: 2px solid var(--color-accent-blue);
    outline-offset: 1px;
  }
</style>
