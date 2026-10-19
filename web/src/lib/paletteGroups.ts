import type { SearchHit } from '../queries/search'

export interface HitGroup {
  kind: string
  hits: SearchHit[]
  total: number
}

const DEFAULT_CAP = 4
const CAPS: Record<string, number> = { action: 6 }

export function capFor(kind: string): number {
  return CAPS[kind] ?? DEFAULT_CAP
}

// Groups appear in the order of their best hit, so relevance stays mixed
// across kinds instead of a fixed kind order.
export function groupHits(
  hits: readonly SearchHit[],
  expanded: ReadonlySet<string>,
): HitGroup[] {
  const order: string[] = []
  const byKind = new Map<string, SearchHit[]>()
  for (const h of hits) {
    const list = byKind.get(h.kind)
    if (list) list.push(h)
    else {
      byKind.set(h.kind, [h])
      order.push(h.kind)
    }
  }
  return order.map((kind) => {
    const all = byKind.get(kind) ?? []
    const shown = expanded.has(kind) ? all : all.slice(0, capFor(kind))
    return { kind, hits: shown, total: all.length }
  })
}

// Characters of `text` covered by any term (substring, case-insensitive).
// The API sends no spans, so matching happens here.
export function highlightIndices(
  text: string,
  terms: readonly string[],
): Set<number> {
  const out = new Set<number>()
  const lower = text.toLowerCase()
  for (const term of terms) {
    if (term === '') continue
    let at = lower.indexOf(term)
    while (at !== -1) {
      for (let i = at; i < at + term.length; i++) out.add(i)
      at = lower.indexOf(term, at + term.length)
    }
  }
  return out
}
