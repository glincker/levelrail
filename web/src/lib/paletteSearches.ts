const KEY = 'palette.recent-searches'
const MAX = 8
const MIN_LENGTH = 3

export function loadRecentSearches(): string[] {
  try {
    const raw = window.localStorage.getItem(KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((v): v is string => typeof v === 'string')
      .slice(0, MAX)
  } catch {
    return []
  }
}

/** A search worth remembering: at least one real word, not a typing fragment or punctuation. */
export function isMeaningfulSearch(q: string): boolean {
  const t = q.trim()
  if (t.length < MIN_LENGTH) return false
  return /[\p{L}\p{N}]{3,}/u.test(t)
}

/**
 * Adds a search at the front. A longer search replaces the shorter one it extends,
 * and a fragment of a search already kept is ignored, so typing "backup" never leaves "ba" behind.
 */
export function withRecentSearch(list: readonly string[], q: string): string[] {
  const t = q.trim()
  if (!isMeaningfulSearch(t)) return [...list]
  const lower = t.toLowerCase()
  if (list.some((s) => s !== t && s.toLowerCase().startsWith(lower))) {
    return [...list]
  }
  const rest = list.filter(
    (s) => s !== t && !lower.startsWith(s.trim().toLowerCase()),
  )
  return [t, ...rest].slice(0, MAX)
}

function save(next: string[]): string[] {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next))
  } catch {
    // Storage unavailable: recent searches just do not persist.
  }
  return next
}

export function pushRecentSearch(q: string): string[] {
  return save(withRecentSearch(loadRecentSearches(), q))
}

export function removeRecentSearch(q: string): string[] {
  return save(loadRecentSearches().filter((s) => s !== q))
}
