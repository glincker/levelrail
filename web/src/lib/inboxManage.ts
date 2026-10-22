import type { InboxEntry } from './inboxEntries'

const MINUTE_MS = 60_000

function envNumber(raw: unknown, fallback: number): number {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

// Seconds the Undo toast stays up after a delete.
export const UNDO_SECONDS = envNumber(
  import.meta.env.VITE_INBOX_UNDO_SECONDS,
  8,
)

// Alerts of one rule and target closer together than this collapse into one row.
export const GROUP_WINDOW_MS =
  envNumber(import.meta.env.VITE_INBOX_GROUP_WINDOW_MINUTES, 60) * MINUTE_MS

export type InboxRow =
  | { type: 'entry'; entry: InboxEntry }
  | { type: 'group'; key: string; entries: InboxEntry[] }

export function rowIds(row: InboxRow): string[] {
  return row.type === 'entry' ? [row.entry.id] : row.entries.map((e) => e.id)
}

// Presentation only: stored items are untouched. Entries must be newest first.
export function groupEntries(
  entries: InboxEntry[],
  windowMs = GROUP_WINDOW_MS,
): InboxRow[] {
  const rows: InboxRow[] = []
  const open = new Map<string, { entries: InboxEntry[]; last: number }>()
  for (const e of entries) {
    const at = e.at ? Date.parse(e.at) : Number.NaN
    const key = e.groupKey
    const cur = key ? open.get(key) : undefined
    if (key && cur && Math.abs(cur.last - at) <= windowMs) {
      cur.entries.push(e)
      cur.last = at
      continue
    }
    if (!key) {
      rows.push({ type: 'entry', entry: e })
      continue
    }
    const group = { entries: [e], last: at }
    open.set(key, group)
    rows.push({ type: 'group', key: `${key}@${e.id}`, entries: group.entries })
  }
  return rows.map((r): InboxRow => {
    const only =
      r.type === 'group' && r.entries.length === 1 ? r.entries[0] : undefined
    return only ? { type: 'entry', entry: only } : r
  })
}

export function selectRange(
  ids: string[],
  anchor: string | null,
  target: string,
): string[] {
  const a = anchor ? ids.indexOf(anchor) : -1
  const b = ids.indexOf(target)
  if (b < 0) return []
  if (a < 0) return [target]
  return ids.slice(Math.min(a, b), Math.max(a, b) + 1)
}

export interface SelectionState {
  selected: ReadonlySet<string>
  anchor: string | null
}

export function toggleSelection(
  state: SelectionState,
  ids: string[],
  target: string,
  shift: boolean,
): SelectionState {
  const next = new Set(state.selected)
  if (shift && state.anchor) {
    const adding = !state.selected.has(target)
    for (const id of selectRange(ids, state.anchor, target)) {
      if (adding) next.add(id)
      else next.delete(id)
    }
  } else if (next.has(target)) {
    next.delete(target)
  } else {
    next.add(target)
  }
  return { selected: next, anchor: target }
}

export function toggleAll(ids: string[], selected: ReadonlySet<string>) {
  const all = ids.length > 0 && ids.every((id) => selected.has(id))
  return all ? new Set<string>() : new Set(ids)
}

export function selectedEntries(
  entries: InboxEntry[],
  selected: ReadonlySet<string>,
): InboxEntry[] {
  return entries.filter((e) => selected.has(e.id))
}

// Delete is offered only when every selected item is a stored alert.
export function canDelete(picked: InboxEntry[]): boolean {
  return picked.length > 0 && picked.every((e) => e.deletable)
}

export interface UndoDeps {
  remove: (ids: string[]) => Promise<void>
  restore: (ids: string[]) => Promise<void>
  notify: (undo: () => Promise<void>) => void
}

// The server hides the items at once; Undo within the toast window restores them.
export async function deleteWithUndo(
  ids: string[],
  deps: UndoDeps,
): Promise<() => Promise<void>> {
  await deps.remove(ids)
  let used = false
  const undo = async () => {
    if (used) return
    used = true
    await deps.restore(ids)
  }
  deps.notify(undo)
  return undo
}
