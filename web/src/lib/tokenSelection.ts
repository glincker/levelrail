import type { TokenFilter } from '../types/tokenList'

// Either an explicit set of ids, or "everything matching this filter" (Gmail style).
export type TokenSelection =
  | { mode: 'ids'; ids: ReadonlySet<string> }
  | { mode: 'filter'; filter: TokenFilter; total: number }

export const EMPTY_SELECTION: TokenSelection = { mode: 'ids', ids: new Set() }

export function selectionCount(sel: TokenSelection): number {
  return sel.mode === 'ids' ? sel.ids.size : sel.total
}

function idSet(sel: TokenSelection): Set<string> {
  return new Set(sel.mode === 'ids' ? sel.ids : [])
}

export function toggleId(sel: TokenSelection, id: string): TokenSelection {
  const ids = idSet(sel)
  if (ids.has(id)) ids.delete(id)
  else ids.add(id)
  return { mode: 'ids', ids }
}

// Shift-click range over the rows currently shown, anchored at the last clicked id.
export function rangeIds(
  order: readonly string[],
  anchor: string,
  target: string,
): string[] {
  const a = order.indexOf(anchor)
  const b = order.indexOf(target)
  if (a < 0 || b < 0) return [target]
  return order.slice(Math.min(a, b), Math.max(a, b) + 1)
}

export function addIds(
  sel: TokenSelection,
  add: readonly string[],
): TokenSelection {
  const ids = idSet(sel)
  for (const id of add) ids.add(id)
  return { mode: 'ids', ids }
}

export function removeIds(
  sel: TokenSelection,
  drop: readonly string[],
): TokenSelection {
  const ids = idSet(sel)
  for (const id of drop) ids.delete(id)
  return { mode: 'ids', ids }
}
