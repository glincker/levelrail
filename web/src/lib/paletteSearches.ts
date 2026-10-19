const KEY = 'palette.recent-searches'
const MAX = 5

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

export function withRecentSearch(list: readonly string[], q: string): string[] {
  const t = q.trim()
  if (t === '') return [...list]
  return [t, ...list.filter((s) => s !== t)].slice(0, MAX)
}

export function pushRecentSearch(q: string): string[] {
  const next = withRecentSearch(loadRecentSearches(), q)
  try {
    window.localStorage.setItem(KEY, JSON.stringify(next))
  } catch {
    // Storage unavailable: recent searches just do not persist.
  }
  return next
}
