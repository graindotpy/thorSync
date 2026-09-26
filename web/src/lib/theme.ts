export type ThemePreference = 'system' | 'paper' | 'ink'

const STORAGE_KEY = 'thorsync.theme'

export function readThemePreference(): ThemePreference {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (stored === 'paper' || stored === 'ink') return stored
  } catch {
    // Storage can be unavailable in private windows; fall back to the system theme.
  }
  return 'system'
}

export function applyThemePreference(preference: ThemePreference): void {
  const root = document.documentElement
  if (preference === 'paper') root.dataset.theme = 'light'
  else if (preference === 'ink') root.dataset.theme = 'dark'
  else delete root.dataset.theme
}

export function saveThemePreference(preference: ThemePreference): void {
  applyThemePreference(preference)
  try {
    if (preference === 'system') window.localStorage.removeItem(STORAGE_KEY)
    else window.localStorage.setItem(STORAGE_KEY, preference)
  } catch {
    // The choice still applies for this visit.
  }
}

export function readLocal<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const stored = window.localStorage.getItem(key)
    if (stored && (allowed as readonly string[]).includes(stored)) return stored as T
  } catch {
    // Ignore unavailable storage.
  }
  return fallback
}

export function writeLocal(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value)
  } catch {
    // Ignore unavailable storage.
  }
}
