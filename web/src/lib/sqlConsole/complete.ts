// Autocomplete candidates from the schema API plus a keyword list.
import type { DbSchemaNode } from '../../types/databaseViewer'

export type SuggestionKind = 'keyword' | 'table' | 'column'

export interface Suggestion {
  label: string
  // Text inserted in place of the current token.
  insert: string
  kind: SuggestionKind
  // Column type, or the owning table for a column.
  detail?: string
}

const KEYWORDS = (
  'SELECT FROM WHERE AND OR NOT IN IS NULL AS ON JOIN LEFT JOIN INNER JOIN GROUP BY ORDER BY HAVING ' +
  'LIMIT OFFSET UNION ALL DISTINCT INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE ALTER DROP WITH ' +
  'RETURNING CASE WHEN THEN ELSE END ASC DESC LIKE ILIKE BETWEEN EXISTS COUNT SUM AVG MIN MAX'
).split(' ')

const MAX_SUGGESTIONS = 8
const IDENT_SAFE = /^[a-z_][a-z0-9_]*$/

function quoteIdent(name: string): string {
  return IDENT_SAFE.test(name) ? name : `"${name.replace(/"/g, '""')}"`
}

// The identifier-ish token ending at the caret, including a dotted prefix.
export function tokenAt(
  text: string,
  caret: number,
): { start: number; token: string } {
  let start = caret
  while (start > 0 && /[A-Za-z0-9_$.]/.test(text.charAt(start - 1))) start--
  return { start, token: text.slice(start, caret) }
}

function rank(label: string, needle: string): number {
  const l = label.toLowerCase()
  if (l === needle) return -1
  if (l.startsWith(needle)) return 0
  if (l.includes(needle)) return 1
  return -1
}

const TABLE_CONTEXT = new Set(['from', 'join', 'into', 'update', 'table'])

// Last keyword-ish word before the token, to tell table slots from column slots.
export function previousWord(text: string, tokenStart: number): string {
  const m = /([A-Za-z_]+)\s+$/.exec(text.slice(0, tokenStart))
  return m ? (m[1] ?? '').toLowerCase() : ''
}

export function suggest(
  token: string,
  schemas: DbSchemaNode[],
  forced = false,
  prev = '',
): Suggestion[] {
  const tableSlot = TABLE_CONTEXT.has(prev)
  if (token === '' && !forced) return []
  const dot = token.lastIndexOf('.')
  const qualifier = dot >= 0 ? token.slice(0, dot).toLowerCase() : ''
  const needle = (dot >= 0 ? token.slice(dot + 1) : token).toLowerCase()
  const pool: Suggestion[] = []

  for (const s of schemas) {
    for (const t of s.tables) {
      const qualified = `${quoteIdent(s.name)}.${quoteIdent(t.name)}`
      if (qualifier === '') {
        pool.push({
          label: t.name,
          insert: quoteIdent(t.name),
          kind: 'table',
          detail: s.name,
        })
        pool.push({
          label: `${s.name}.${t.name}`,
          insert: qualified,
          kind: 'table',
          detail: t.kind,
        })
      } else if (qualifier === s.name.toLowerCase()) {
        pool.push({
          label: t.name,
          insert: qualified,
          kind: 'table',
          detail: t.kind,
        })
      }
      if (
        !tableSlot &&
        (qualifier === '' ||
          qualifier === t.name.toLowerCase() ||
          qualifier === `${s.name}.${t.name}`.toLowerCase())
      ) {
        for (const c of t.columns) {
          pool.push({
            label: c.name,
            insert:
              qualifier === ''
                ? quoteIdent(c.name)
                : `${token.slice(0, dot + 1)}${quoteIdent(c.name)}`,
            kind: 'column',
            detail: `${c.type} | ${t.name}`,
          })
        }
      }
    }
  }
  if (qualifier === '' && !tableSlot) {
    for (const k of KEYWORDS)
      pool.push({ label: k, insert: k, kind: 'keyword' })
  }

  const seen = new Set<string>()
  const scored: { s: Suggestion; r: number; order: number }[] = []
  pool.forEach((s, order) => {
    const r = needle === '' ? 0 : rank(s.label, needle)
    const key = `${s.kind}|${s.insert}|${s.detail ?? ''}`
    if ((r < 0 && needle !== '') || seen.has(key)) return
    // An exact keyword echo adds nothing.
    if (s.label.toLowerCase() === needle && s.kind === 'keyword') return
    seen.add(key)
    scored.push({ s, r, order })
  })
  const kindOrder: Record<SuggestionKind, number> = {
    column: 0,
    table: 1,
    keyword: 2,
  }
  scored.sort(
    (a, b) =>
      a.r - b.r ||
      kindOrder[a.s.kind] - kindOrder[b.s.kind] ||
      a.order - b.order,
  )
  return scored.slice(0, MAX_SUGGESTIONS).map((x) => x.s)
}
