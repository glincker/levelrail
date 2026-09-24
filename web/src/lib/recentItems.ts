const STORAGE_KEY = 'palette.recent'
const MAX_RECENT = 5

/** Reads recent palette item keys, newest first. Never throws. */
export function loadRecentKeys(): string[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((v): v is string => typeof v === 'string')
      .slice(0, MAX_RECENT)
  } catch {
    return []
  }
}

/** Moves `key` to the front, dedupes and caps the list. */
export function withRecent(keys: readonly string[], key: string): string[] {
  return [key, ...keys.filter((k) => k !== key)].slice(0, MAX_RECENT)
}

/** Records `key` as most recent and returns the new list. Never throws. */
export function pushRecentKey(key: string): string[] {
  const next = withRecent(loadRecentKeys(), key)
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    // Storage unavailable: recents just do not persist.
  }
  return next
}
