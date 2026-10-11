const WORD_BREAK = /[\s\-_/.]/

/**
 * Scores how well `query` matches `text` as an ordered subsequence.
 * Returns null when it does not match; higher is better.
 */
export function fuzzyScore(query: string, text: string): number | null {
  const q = query.trim().toLowerCase()
  if (!q) return 0
  const t = text.toLowerCase()

  const at = t.indexOf(q)
  if (at !== -1) {
    const wordStart = at === 0 || WORD_BREAK.test(t.charAt(at - 1))
    return 100 + (at === 0 ? 50 : 0) + (wordStart ? 20 : 0) - t.length
  }

  let score = 0
  let ti = 0
  let prev = -2
  for (const ch of q) {
    const found = t.indexOf(ch, ti)
    if (found === -1) return null
    if (found === prev + 1) score += 5
    else if (found === 0 || WORD_BREAK.test(t.charAt(found - 1))) score += 3
    else score += 1
    prev = found
    ti = found + 1
  }
  return score - t.length / 10
}

// Keyword-only hits rank below label hits so an app name never outranks the
// domain you typed.
const KEYWORD_PENALTY = 30

/**
 * Filters to matching items and sorts best first; empty query keeps order.
 * `getKeywords` adds a lower-ranked second chance (e.g. a domain's app name).
 */
export function fuzzyFilter<T>(
  items: readonly T[],
  query: string,
  getText: (item: T) => string,
  getKeywords?: (item: T) => string | undefined,
): T[] {
  if (!query.trim()) return [...items]
  const scored: Array<{ item: T; score: number; index: number }> = []
  items.forEach((item, index) => {
    let score = fuzzyScore(query, getText(item))
    if (score === null) {
      const keywords = getKeywords?.(item)
      const hit = keywords ? fuzzyScore(query, keywords) : null
      score = hit === null ? null : hit - KEYWORD_PENALTY
    }
    if (score !== null) scored.push({ item, score, index })
  })
  scored.sort((a, b) => b.score - a.score || a.index - b.index)
  return scored.map((s) => s.item)
}

/** Character positions in `text` that matched `query`, for highlighting. */
export function fuzzyMatchIndices(query: string, text: string): number[] {
  const q = query.trim().toLowerCase()
  if (!q) return []
  const t = text.toLowerCase()
  const at = t.indexOf(q)
  if (at !== -1) return Array.from({ length: q.length }, (_, i) => at + i)
  const out: number[] = []
  let ti = 0
  for (const ch of q) {
    const found = t.indexOf(ch, ti)
    if (found === -1) return []
    out.push(found)
    ti = found + 1
  }
  return out
}
