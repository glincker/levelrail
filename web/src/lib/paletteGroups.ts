import type { SearchHit, SearchSpan } from '../queries/search'

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

// Code point indices covered by the server's title spans.
export function spanIndices(
  spans: readonly SearchSpan[] | undefined,
): Set<number> {
  const out = new Set<number>()
  for (const s of spans ?? []) {
    for (let i = s.start; i < s.end; i++) out.add(i)
  }
  return out
}
