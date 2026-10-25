import { ITEM_KEYWORDS, SYNONYM_GROUPS } from './paletteSearchIndex'

const WORD_BREAK = /[\s\-_/.,:]+/

// Tier bases leave room for in-tier tie breaks (shorter text wins).
const PREFIX = 1000
const WORD_PREFIX = 700
const SUBSTRING = 450
const SUBSEQUENCE = 200
const KEYWORD_EXACT = 420
const KEYWORD_PREFIX = 380
const KEYWORD_SUBSTRING = 260

function words(text: string): string[] {
  return text.toLowerCase().split(WORD_BREAK).filter(Boolean)
}

function isSubsequence(q: string, t: string): boolean {
  let at = 0
  for (const ch of q) {
    at = t.indexOf(ch, at)
    if (at === -1) return false
    at++
  }
  return true
}

/** Scores one query token against label text: prefix > word > substring > subsequence. */
export function scoreToken(token: string, text: string): number | null {
  const q = token.toLowerCase()
  const t = text.toLowerCase()
  if (!q) return 0
  const tie = Math.max(0, 40 - t.length)
  if (t.startsWith(q)) return PREFIX + tie
  if (words(t).some((w) => w.startsWith(q))) return WORD_PREFIX + tie
  if (t.includes(q)) return SUBSTRING + tie
  if (q.length >= 2 && isSubsequence(q, t)) return SUBSEQUENCE + tie
  return null
}

/** Scores a token against keyword text (whole-word matches beat partials). */
export function scoreKeywordToken(
  token: string,
  keywords: string,
): number | null {
  const q = token.toLowerCase()
  const ws = words(keywords)
  const at = ws.indexOf(q)
  if (at !== -1) return KEYWORD_EXACT - Math.min(at, 15)
  if (ws.some((w) => w.startsWith(q))) return KEYWORD_PREFIX
  if (q.length >= 3 && ws.some((w) => w.includes(q))) return KEYWORD_SUBSTRING
  return null
}

/**
 * Scores a multi-word query. Every word must match the label or the keywords;
 * the better of the two wins per word. Returns null when any word misses.
 */
export function scoreItem(
  query: string,
  label: string,
  keywords?: string,
): number | null {
  const tokens = query.trim().split(/\s+/).filter(Boolean)
  if (tokens.length === 0) return 0
  let total = 0
  for (const token of tokens) {
    const l = scoreToken(token, label)
    const k = keywords ? scoreKeywordToken(token, keywords) : null
    const best = Math.max(l ?? -1, k ?? -1)
    if (best < 0) return null
    total += best
  }
  return total / tokens.length
}

/** Item keywords: whatever the item carries plus the shared synonym index. */
export function keywordsFor(key: string, own?: string): string {
  const stripped = key.replace(/^(recent-|suggested-quick-)/, '')
  return [own, ITEM_KEYWORDS[stripped]].filter(Boolean).join(' ')
}

/** Filters to matches and sorts best first; empty query keeps the order. */
export function rankItems<T extends { key: string; label: string }>(
  items: readonly T[],
  query: string,
  getKeywords?: (item: T) => string | undefined,
): T[] {
  if (!query.trim()) return [...items]
  const scored: Array<{ item: T; score: number; index: number }> = []
  items.forEach((item, index) => {
    const score = scoreItem(
      query,
      item.label,
      keywordsFor(item.key, getKeywords?.(item)),
    )
    if (score !== null) scored.push({ item, score, index })
  })
  scored.sort((a, b) => b.score - a.score || a.index - b.index)
  return scored.map((s) => s.item)
}

export interface Related {
  keys: string[]
  templates: string[]
}

/** Related item keys and template hints for what the user is typing. */
export function relatedFor(query: string): Related {
  const tokens = query.toLowerCase().split(/\s+/).filter(Boolean)
  const keys: string[] = []
  const templates: string[] = []
  for (const g of SYNONYM_GROUPS) {
    const hit = tokens.some(
      (tok) => tok.length >= 2 && g.terms.some((term) => term.startsWith(tok)),
    )
    if (!hit) continue
    for (const k of g.keys) if (!keys.includes(k)) keys.push(k)
    for (const tpl of g.templates ?? []) {
      if (!templates.includes(tpl)) templates.push(tpl)
    }
  }
  return { keys, templates }
}
