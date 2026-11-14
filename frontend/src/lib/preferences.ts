import { DEFAULT_DARK_THEME, DEFAULT_LIGHT_THEME, isThemeId, type Appearance, type ThemeId } from './themes'

export interface Preferences {
  theme: Appearance
  lightTheme: ThemeId
  darkTheme: ThemeId
  model: string
  enterToSend: boolean
}
export const DEFAULT_PREFERENCES: Preferences = {
  theme: 'system', lightTheme: DEFAULT_LIGHT_THEME, darkTheme: DEFAULT_DARK_THEME,
  model: 'qwen2.5:3b', enterToSend: true,
}
// Older preferences have only theme/model/enterToSend. Preserve them while adding
// independent palettes; stale or invalid palette IDs fall back to the defaults.
export function normalizePreferences(value: unknown): Preferences {
  const saved = value && typeof value === 'object' ? value as Record<string, unknown> : {}
  return {
    theme: saved.theme === 'light' || saved.theme === 'dark' || saved.theme === 'system' ? saved.theme : DEFAULT_PREFERENCES.theme,
    lightTheme: isThemeId(saved.lightTheme) ? saved.lightTheme : DEFAULT_PREFERENCES.lightTheme,
    darkTheme: isThemeId(saved.darkTheme) ? saved.darkTheme : DEFAULT_PREFERENCES.darkTheme,
    model: typeof saved.model === 'string' && saved.model.trim() ? saved.model : DEFAULT_PREFERENCES.model,
    enterToSend: typeof saved.enterToSend === 'boolean' ? saved.enterToSend : DEFAULT_PREFERENCES.enterToSend,
  }
}
