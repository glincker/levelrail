import {
  NAV_PREFS_STORAGE_KEY,
  NAV_RECENT_MAX,
  NAV_RECENT_STORAGE_KEY,
} from './navConfig'

export { NAV_PREFS_STORAGE_KEY, NAV_RECENT_STORAGE_KEY }

export interface NavPrefs {
  hiddenItems: string[]
  hiddenGroups: string[]
  groupOrder: string[]
  itemOrder: Record<string, string[]>
}

export const EMPTY_NAV_PREFS: NavPrefs = {
  hiddenItems: [],
  hiddenGroups: [],
  groupOrder: [],
  itemOrder: {},
}

interface PrefGroup<I extends { id: string }> {
  id: string
  ungrouped?: boolean
  items: I[]
}

function strings(v: unknown): string[] {
  if (!Array.isArray(v)) return []
  const out: string[] = []
  for (const x of v) {
    if (typeof x === 'string' && x !== '' && !out.includes(x)) out.push(x)
  }
  return out
}

// A corrupt stored value yields the defaults so it can never break the sidebar.
export function parseNavPrefs(raw: string | null): NavPrefs {
  if (!raw) return EMPTY_NAV_PREFS
  try {
    const p: unknown = JSON.parse(raw)
    if (typeof p !== 'object' || p === null) return EMPTY_NAV_PREFS
    const rec = p as Record<string, unknown>
    const itemOrder: Record<string, string[]> = {}
    const io = rec.itemOrder
    if (typeof io === 'object' && io !== null && !Array.isArray(io)) {
      for (const [k, v] of Object.entries(io)) itemOrder[k] = strings(v)
    }
    return {
      hiddenItems: strings(rec.hiddenItems),
      hiddenGroups: strings(rec.hiddenGroups),
      groupOrder: strings(rec.groupOrder),
      itemOrder,
    }
  } catch {
    return EMPTY_NAV_PREFS
  }
}

export function isDefaultNavPrefs(p: NavPrefs): boolean {
  return (
    p.hiddenItems.length === 0 &&
    p.hiddenGroups.length === 0 &&
    p.groupOrder.length === 0 &&
    Object.keys(p.itemOrder).length === 0
  )
}

// Saved ids first in saved order; anything new keeps its default place after them.
export function orderIds(
  ids: readonly string[],
  saved: readonly string[],
): string[] {
  const known = saved.filter((id) => ids.includes(id))
  return [...known, ...ids.filter((id) => !known.includes(id))]
}

export function moveId(
  ids: readonly string[],
  id: string,
  delta: -1 | 1,
): string[] {
  const from = ids.indexOf(id)
  const to = from + delta
  if (from < 0 || to < 0 || to >= ids.length) return [...ids]
  const next = [...ids]
  next.splice(from, 1)
  next.splice(to, 0, id)
  return next
}

// Drag and drop: the dragged id takes the slot of the target id.
export function moveIdTo(
  ids: readonly string[],
  id: string,
  targetId: string,
): string[] {
  const from = ids.indexOf(id)
  const to = ids.indexOf(targetId)
  if (from < 0 || to < 0 || from === to) return [...ids]
  const next = [...ids]
  next.splice(from, 1)
  next.splice(to, 0, id)
  return next
}

export function groupHasItem<I extends { id: string }>(
  group: PrefGroup<I>,
  itemId: string | undefined,
): boolean {
  return itemId !== undefined && group.items.some((i) => i.id === itemId)
}

// The current page is always reachable, so its item and its group cannot be hidden.
export function canHideItem(id: string, activeId: string | undefined) {
  return id !== activeId
}

export function canHideGroup<I extends { id: string }>(
  group: PrefGroup<I>,
  activeId: string | undefined,
) {
  return !group.ungrouped && !groupHasItem(group, activeId)
}

export function applyNavPrefs<I extends { id: string }, G extends PrefGroup<I>>(
  groups: readonly G[],
  prefs: NavPrefs,
  activeId?: string,
): G[] {
  const ungrouped = groups.filter((g) => g.ungrouped)
  const rest = groups.filter((g) => !g.ungrouped)
  const order = orderIds(
    rest.map((g) => g.id),
    prefs.groupOrder,
  )
  const ordered = order.flatMap((id) => rest.filter((g) => g.id === id))
  const out: G[] = []
  for (const g of [...ungrouped, ...ordered]) {
    if (
      !g.ungrouped &&
      prefs.hiddenGroups.includes(g.id) &&
      !groupHasItem(g, activeId)
    ) {
      continue
    }
    const ids = orderIds(
      g.items.map((i) => i.id),
      prefs.itemOrder[g.id] ?? [],
    )
    const items = ids
      .flatMap((id) => g.items.filter((i) => i.id === id))
      .filter((i) => !prefs.hiddenItems.includes(i.id) || i.id === activeId)
    if (items.length > 0 || g.items.length === 0) out.push({ ...g, items })
  }
  return out
}

export function withItemHidden(
  prefs: NavPrefs,
  id: string,
  hidden: boolean,
  activeId: string | undefined,
): NavPrefs {
  if (hidden && !canHideItem(id, activeId)) return prefs
  const rest = prefs.hiddenItems.filter((x) => x !== id)
  return { ...prefs, hiddenItems: hidden ? [...rest, id] : rest }
}

export function withGroupHidden(
  prefs: NavPrefs,
  id: string,
  hidden: boolean,
): NavPrefs {
  const rest = prefs.hiddenGroups.filter((x) => x !== id)
  return { ...prefs, hiddenGroups: hidden ? [...rest, id] : rest }
}

export function withItemOrder(
  prefs: NavPrefs,
  groupId: string,
  ids: readonly string[],
): NavPrefs {
  return { ...prefs, itemOrder: { ...prefs.itemOrder, [groupId]: [...ids] } }
}

export function withGroupOrder(
  prefs: NavPrefs,
  ids: readonly string[],
): NavPrefs {
  return { ...prefs, groupOrder: [...ids] }
}

export type RecentKind = 'app' | 'database'

export interface RecentEntry {
  kind: RecentKind
  name: string
}

const RECENT_PATH = /^\/(apps|databases)\/([^/]+)/
// Static sub-routes that share the prefix but are not a named detail page.
const NOT_A_NAME = new Set(['new', 'create', 'import'])

export function recentFromPath(pathname: string): RecentEntry | undefined {
  const m = RECENT_PATH.exec(pathname)
  if (!m) return undefined
  let name: string
  try {
    name = decodeURIComponent(m[2] ?? '')
  } catch {
    return undefined
  }
  if (name === '' || NOT_A_NAME.has(name)) return undefined
  return { kind: m[1] === 'apps' ? 'app' : 'database', name }
}

export function pushRecent(
  list: readonly RecentEntry[],
  entry: RecentEntry,
  max = NAV_RECENT_MAX,
): RecentEntry[] {
  const rest = list.filter(
    (e) => !(e.kind === entry.kind && e.name === entry.name),
  )
  return [entry, ...rest].slice(0, max)
}

// Returns the new stored value, or null when the visit changes nothing.
export function recordRecent(
  pathname: string,
  raw: string | null,
): string | null {
  const entry = recentFromPath(pathname)
  if (!entry) return null
  const next = JSON.stringify(pushRecent(parseRecent(raw), entry))
  return next === raw ? null : next
}

export function parseRecent(raw: string | null): RecentEntry[] {
  if (!raw) return []
  try {
    const p: unknown = JSON.parse(raw)
    if (!Array.isArray(p)) return []
    const out: RecentEntry[] = []
    for (const v of p) {
      if (typeof v !== 'object' || v === null) continue
      const r = v as Record<string, unknown>
      if (
        (r.kind === 'app' || r.kind === 'database') &&
        typeof r.name === 'string' &&
        r.name !== ''
      ) {
        out.push({ kind: r.kind, name: r.name })
      }
    }
    return out.slice(0, NAV_RECENT_MAX)
  } catch {
    return []
  }
}
