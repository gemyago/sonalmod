import { describe, it, expect } from 'vitest'
import { effectiveTheme, parseStoredPreference } from './theme'

describe('theme', () => {
  it('effectiveTheme respects explicit light and dark', () => {
    expect(effectiveTheme('light', true)).toBe('light')
    expect(effectiveTheme('light', false)).toBe('light')
    expect(effectiveTheme('dark', true)).toBe('dark')
    expect(effectiveTheme('dark', false)).toBe('dark')
  })

  it('effectiveTheme auto follows prefersDark', () => {
    expect(effectiveTheme('auto', true)).toBe('dark')
    expect(effectiveTheme('auto', false)).toBe('light')
  })

  it('parseStoredPreference defaults invalid or missing to auto', () => {
    expect(parseStoredPreference(null)).toBe('auto')
    expect(parseStoredPreference('')).toBe('auto')
    expect(parseStoredPreference('light')).toBe('light')
    expect(parseStoredPreference('bogus')).toBe('auto')
  })
})
