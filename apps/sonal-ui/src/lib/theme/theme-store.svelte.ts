import {
  applyDomTheme,
  effectiveTheme,
  prefersColorSchemeDark,
  readStoredPreference,
  writeStoredPreference,
  type ThemePreference,
} from './theme'

export class ThemeStore {
  preference = $state<ThemePreference>('auto')
  private mediaUnsub: (() => void) | null = null

  constructor() {
    if (typeof window === 'undefined') return
    this.preference = readStoredPreference()
    this.syncDom()
    this.attachMediaListener()
  }

  /** Effective appearance for the current preference and system setting. */
  get effective(): 'light' | 'dark' {
    return effectiveTheme(this.preference, prefersColorSchemeDark())
  }

  setPreference(pref: ThemePreference): void {
    this.preference = pref
    writeStoredPreference(pref)
    this.syncDom()
    this.attachMediaListener()
  }

  /** Re-read storage (e.g. after bootstrap) and sync. */
  hydrateFromStorage(): void {
    if (typeof window === 'undefined') return
    this.preference = readStoredPreference()
    this.syncDom()
    this.attachMediaListener()
  }

  private syncDom(): void {
    applyDomTheme(effectiveTheme(this.preference, prefersColorSchemeDark()))
  }

  private attachMediaListener(): void {
    this.mediaUnsub?.()
    this.mediaUnsub = null
    if (typeof window === 'undefined' || this.preference !== 'auto') return
    if (typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (): void => {
      this.syncDom()
    }
    mq.addEventListener('change', onChange)
    this.mediaUnsub = () => mq.removeEventListener('change', onChange)
  }
}

export const themeStore = new ThemeStore()

export type { ThemePreference }
