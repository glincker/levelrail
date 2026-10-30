// Local-only palette history: how often and how recently each destination is opened, and what tends
// to follow what. It never leaves this browser; clearHistory() wipes it.
const KEY = 'palette.history'
const HALF_LIFE_MS = 14 * 24 * 60 * 60 * 1000
const MAX_ITEMS = 200

interface History {
  items: Record<string, { n: number; t: number }>
  next: Record<string, Record<string, number>>
  last?: string
}

const KEY_PREFIXES = ['recent-', 'suggested-quick-', 'related-']

/** The key an item has in the base list, without the prefix a group adds. */
export function normalizeKey(key: string): string {
  for (const prefix of KEY_PREFIXES) {
    if (key.startsWith(prefix)) return normalizeKey(key.slice(prefix.length))
  }
  return key
}

function load(): History {
  try {
    const raw = window.localStorage.getItem(KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : null
    if (parsed && typeof parsed === 'object') {
      const h = parsed as Partial<History>
      return { items: h.items ?? {}, next: h.next ?? {}, last: h.last }
    }
  } catch {
    // fall through to an empty history
  }
  return { items: {}, next: {} }
}

function score(entry: { n: number; t: number }, now: number): number {
  return entry.n * Math.pow(0.5, (now - entry.t) / HALF_LIFE_MS)
}

/** Counts an opened destination and the step from the previous one. */
export function recordVisit(rawKey: string, now = Date.now()): void {
  const key = normalizeKey(rawKey)
  const h = load()
  const entry = h.items[key] ?? { n: 0, t: now }
  h.items[key] = { n: entry.n + 1, t: now }
  if (h.last && h.last !== key) {
    const row = h.next[h.last] ?? {}
    row[key] = (row[key] ?? 0) + 1
    h.next[h.last] = row
  }
  h.last = key
  const keys = Object.keys(h.items)
  if (keys.length > MAX_ITEMS) {
    const keep = new Set(
      keys
        .sort((a, b) => score(h.items[b]!, now) - score(h.items[a]!, now))
        .slice(0, MAX_ITEMS),
    )
    for (const k of keys) if (!keep.has(k)) delete h.items[k]
  }
  try {
    window.localStorage.setItem(KEY, JSON.stringify(h))
  } catch {
    // Storage unavailable: history just does not persist.
  }
}

/** The most used destinations, weighing recent use more, at least two opens each. */
export function topFrecent(count: number, now = Date.now()): string[] {
  const h = load()
  return Object.entries(h.items)
    .filter(([, e]) => e.n >= 2)
    .sort((a, b) => score(b[1], now) - score(a[1], now))
    .slice(0, count)
    .map(([k]) => k)
}

/** What you usually open after the destination you opened last. */
export function nextUp(count: number): string[] {
  const h = load()
  const row = h.last ? h.next[h.last] : undefined
  if (!row) return []
  return Object.entries(row)
    .filter(([, n]) => n >= 2)
    .sort((a, b) => b[1] - a[1])
    .slice(0, count)
    .map(([k]) => k)
}

/** Removes the history and both recent lists. */
export function clearHistory(): void {
  try {
    window.localStorage.removeItem(KEY)
    window.localStorage.removeItem('palette.recent')
    window.localStorage.removeItem('palette.recent-searches')
  } catch {
    // nothing to clear
  }
}
