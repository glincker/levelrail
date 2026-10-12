export type ColumnPriority = 1 | 2 | 3

export const DEFAULT_VIRTUALIZE_OVER = 50

// Static strings so Tailwind sees them. Thresholds are widths of the named
// `rtable` container, not the viewport.
const PRIORITY_CLASS: Record<ColumnPriority, string> = {
  1: '',
  2: 'hidden @2xl/rtable:table-cell',
  3: 'hidden @4xl/rtable:table-cell',
}

export function priorityClass(priority: ColumnPriority | undefined): string {
  return PRIORITY_CLASS[priority ?? 1]
}

export function shouldVirtualize(
  count: number,
  threshold: number = DEFAULT_VIRTUALIZE_OVER,
): boolean {
  return count > threshold
}

interface ColumnLike {
  id: string
  primary?: boolean
  hideInCard?: boolean
}

// The first column flagged primary wins; otherwise the first column.
export function splitColumns<C extends ColumnLike>(
  columns: readonly C[],
): { primary: C | undefined; rest: C[] } {
  const primary = columns.find((c) => c.primary) ?? columns[0]
  return { primary, rest: columns.filter((c) => c !== primary) }
}

export function cardFields<C extends ColumnLike>(columns: readonly C[]): C[] {
  return splitColumns(columns).rest.filter((c) => !c.hideInCard)
}

export function paddingFor(
  items: readonly { start: number; end: number }[],
  total: number,
): { top: number; bottom: number } {
  const first = items[0]
  const last = items[items.length - 1]
  if (!first || !last) return { top: 0, bottom: 0 }
  return { top: first.start, bottom: Math.max(0, total - last.end) }
}
