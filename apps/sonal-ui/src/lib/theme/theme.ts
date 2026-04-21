export type ThemePreference = 'auto' | 'light' | 'dark'

export const THEME_STORAGE_KEY = 'sonal-ui-theme'

/** Pure resolver for tests and SSR-safe logic. */
export function effectiveTheme(pref: ThemePreference, prefersDark: boolean): 'light' | 'dark' {
  if (pref === 'light') return 'light'
  if (pref === 'dark') return 'dark'
  return prefersDark ? 'dark' : 'light'
}

export function parseStoredPreference(raw: string | null): ThemePreference {
  if (raw === 'light' || raw === 'dark' || raw === 'auto') return raw
  return 'auto'
}

export function readStoredPreference(): ThemePreference {
  if (typeof localStorage === 'undefined') return 'auto'
  return parseStoredPreference(localStorage.getItem(THEME_STORAGE_KEY))
}

export function writeStoredPreference(pref: ThemePreference): void {
  localStorage.setItem(THEME_STORAGE_KEY, pref)
}

export function applyDomTheme(effective: 'light' | 'dark'): void {
  document.documentElement.dataset.theme = effective
  document.documentElement.style.colorScheme = effective
}

export function prefersColorSchemeDark(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

/** Call once at startup (before mount) so `data-theme` exists for first paint. */
export function bootstrapThemeDom(): ThemePreference {
  const pref = readStoredPreference()
  applyDomTheme(effectiveTheme(pref, prefersColorSchemeDark()))
  return pref
}
