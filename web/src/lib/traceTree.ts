import type { TraceSpan } from '../types/traces'

export interface SpanRow {
  span: TraceSpan
  depth: number
  // Offset and width as a share of the whole trace, 0 to 1.
  left: number
  width: number
}

/**
 * layoutTrace orders spans depth first under their parents and places each on
 * a shared time axis. A span whose parent was not received is a root, so a
 * partial trace still renders.
 */
export function layoutTrace(spans: TraceSpan[]): {
  rows: SpanRow[]
  totalNs: number
} {
  if (spans.length === 0) return { rows: [], totalNs: 0 }
  const ids = new Set(spans.map((s) => s.span_id))
  const children = new Map<string, TraceSpan[]>()
  const roots: TraceSpan[] = []
  for (const s of spans) {
    if (s.parent_id && ids.has(s.parent_id)) {
      const list = children.get(s.parent_id) ?? []
      list.push(s)
      children.set(s.parent_id, list)
    } else {
      roots.push(s)
    }
  }
  const byStart = (a: TraceSpan, b: TraceSpan) => a.start_ns - b.start_ns
  roots.sort(byStart)
  const origin = Math.min(...spans.map((s) => s.start_ns))
  const end = Math.max(...spans.map((s) => s.start_ns + s.duration_ns))
  const totalNs = Math.max(1, end - origin)

  const rows: SpanRow[] = []
  const walk = (s: TraceSpan, depth: number) => {
    rows.push({
      span: s,
      depth,
      left: (s.start_ns - origin) / totalNs,
      width: Math.max(s.duration_ns / totalNs, 0.002),
    })
    for (const c of (children.get(s.span_id) ?? []).sort(byStart)) {
      walk(c, depth + 1)
    }
  }
  for (const r of roots) walk(r, 0)
  return { rows, totalNs }
}

/** formatDuration renders nanoseconds as us, ms or s. */
export function formatDuration(ns: number): string {
  if (ns < 1_000) return `${ns}ns`
  if (ns < 1_000_000) return `${(ns / 1_000).toFixed(0)}us`
  if (ns < 1_000_000_000) return `${(ns / 1_000_000).toFixed(1)}ms`
  return `${(ns / 1_000_000_000).toFixed(2)}s`
}
