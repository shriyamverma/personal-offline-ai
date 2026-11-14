import { describe, expect, it } from 'vitest'
import { DEFAULT_PREFERENCES, normalizePreferences } from './preferences'

describe('saved appearance preferences', () => {
  it('migrates older preferences while preserving appearance, model, and input behavior', () => {
    expect(normalizePreferences({ theme: 'dark', model: 'llama3.2:3b', enterToSend: false })).toEqual({
      ...DEFAULT_PREFERENCES, theme: 'dark', model: 'llama3.2:3b', enterToSend: false,
    })
  })
  it('keeps the light and dark palette choices independent', () => {
    expect(normalizePreferences({ lightTheme: 'paper', darkTheme: 'graphite' })).toMatchObject({ lightTheme: 'paper', darkTheme: 'graphite' })
  })
  it('recovers safely from stale palette IDs and malformed preferences', () => {
    for (const value of [null, 'bad', { theme: 'old-mode', lightTheme: 'missing', darkTheme: null, enterToSend: 'false', model: '' }]) {
      expect(normalizePreferences(value)).toEqual(DEFAULT_PREFERENCES)
    }
  })
})
