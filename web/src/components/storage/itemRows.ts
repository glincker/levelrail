import type { ImageEntry } from '../../queries/imageInventory'
import type { StorageInventory } from '../../queries/storageInventory'

export type ItemKind = 'image' | 'volume' | 'container' | 'buildCache'
export type StorageFilter =
  'appData' | 'images' | 'buildCache' | 'containersLogs' | 'other'
export type ItemView = 'type' | 'app'
export type SortKey = 'size' | 'name'

export const ITEM_KINDS: readonly ItemKind[] = [
  'image',
  'volume',
  'container',
  'buildCache',
]

const FILTERS: readonly StorageFilter[] = [
  'appData',
  'images',
  'buildCache',
  'containersLogs',
  'other',
]

// Server text is shown as is; the other codes are translated by the list.
export type ReasonCode =
  | 'text'
  | 'none'
  | 'orphaned'
  | 'inUse'
  | 'inUseBy'
  | 'unused'
  | 'stopped'
  | 'databaseProtected'
  | 'buildCache'

export interface ItemReason {
  code: ReasonCode
  text?: string
  app?: string
}

export interface StorageItemRow {
  id: string
  kind: ItemKind
  name: string
  /** True for ids and digests, which read better in a monospace face. */
  mono: boolean
  bytes: number | null
  ownerApp: string | null
  reason: ItemReason
  tone: 'neutral' | 'amber'
  protected: boolean
  lastAt?: string
  filter: StorageFilter
}

export interface ItemGroup {
  key: string
  kind: ItemKind | null
  app: string | null
  bytes: number
  counts: Record<ItemKind, number>
  lastAt?: string
  rows: StorageItemRow[]
}

export function parseView(raw: string | undefined): ItemView {
  return raw === 'app' ? 'app' : 'type'
}

export function parseFilter(raw: string | undefined): StorageFilter | null {
  return FILTERS.find((f) => f === raw) ?? null
}

function shortId(id: string): string {
  return id.replace(/^sha256:/, '').slice(0, 12)
}

function latest(
  a: string | undefined,
  b: string | undefined,
): string | undefined {
  if (!a) return b
  if (!b) return a
  return a > b ? a : b
}

function imageReason(e: ImageEntry, app: string | null): ItemReason {
  if (e.blocked_reason) return { code: 'text', text: e.blocked_reason }
  if (e.protected && e.protect_reason)
    return { code: 'text', text: e.protect_reason }
  if (e.removable_reason) return { code: 'text', text: e.removable_reason }
  if (e.in_use && app) return { code: 'inUseBy', app }
  if (e.in_use) return { code: 'inUse' }
  return { code: 'none' }
}

export function imageRow(e: ImageEntry, largeBytes: number): StorageItemRow {
  const app = e.apps[0]?.app ?? null
  const lastAt = e.apps.reduce<string | undefined>(
    (acc, a) => latest(acc, a.at),
    undefined,
  )
  const tag = e.tags[0]
  const blocked = Boolean(e.blocked_reason)
  const largeUnmanaged = e.unmanaged && e.size_bytes >= largeBytes
  return {
    id: `image:${e.id}`,
    kind: 'image',
    name: tag ?? shortId(e.id),
    mono: !tag,
    bytes: e.size_bytes,
    ownerApp: app,
    reason: imageReason(e, app),
    tone: blocked || largeUnmanaged ? 'amber' : 'neutral',
    protected: e.protected,
    lastAt,
    filter: 'images',
  }
}

function volumeReason(
  v: StorageInventory['volumes'][number],
  dbLocked: boolean,
): ItemReason {
  if (v.protected && v.protect_reason)
    return { code: 'text', text: v.protect_reason }
  if (dbLocked) return { code: 'databaseProtected' }
  if (v.orphaned) return { code: 'orphaned' }
  if (v.in_use && v.owner_app) return { code: 'inUseBy', app: v.owner_app }
  if (v.in_use) return { code: 'inUse' }
  return { code: 'unused' }
}

export function buildItemRows(input: {
  images: ImageEntry[]
  inventory: StorageInventory | undefined
  buildCacheBytes: number | null | undefined
  largeBytes: number
}): StorageItemRow[] {
  const rows: StorageItemRow[] = input.images.map((e) =>
    imageRow(e, input.largeBytes),
  )
  for (const v of input.inventory?.volumes ?? []) {
    const dbLocked = v.is_database && v.protected
    rows.push({
      id: `volume:${v.name}`,
      kind: 'volume',
      name: v.name,
      mono: true,
      bytes: v.size_bytes ?? null,
      ownerApp: v.owner_app ?? null,
      reason: volumeReason(v, dbLocked),
      tone: dbLocked ? 'amber' : 'neutral',
      protected: v.protected,
      filter: v.orphaned || !v.owner_app ? 'other' : 'appData',
    })
  }
  for (const c of input.inventory?.containers ?? []) {
    rows.push({
      id: `container:${c.id}`,
      kind: 'container',
      name: c.name || shortId(c.id),
      mono: !c.name,
      bytes: c.size_bytes ?? null,
      ownerApp: c.owner_app ?? null,
      reason: { code: 'stopped' },
      tone: 'neutral',
      protected: false,
      lastAt: c.exited_at,
      filter: 'containersLogs',
    })
  }
  if (input.buildCacheBytes && input.buildCacheBytes > 0) {
    rows.push({
      id: 'buildCache:local',
      kind: 'buildCache',
      name: 'build-cache',
      mono: false,
      bytes: input.buildCacheBytes,
      ownerApp: null,
      reason: { code: 'buildCache' },
      tone: 'neutral',
      protected: false,
      filter: 'buildCache',
    })
  }
  return rows
}

export function filterRows(
  rows: StorageItemRow[],
  filter: StorageFilter | null,
  query: string,
): StorageItemRow[] {
  const q = query.trim().toLowerCase()
  return rows.filter((r) => {
    if (filter && r.filter !== filter) return false
    if (!q) return true
    return (
      r.name.toLowerCase().includes(q) ||
      r.id.toLowerCase().includes(q) ||
      (r.ownerApp ?? '').toLowerCase().includes(q)
    )
  })
}

export function sortRows(
  rows: StorageItemRow[],
  sort: SortKey,
): StorageItemRow[] {
  const out = [...rows]
  if (sort === 'name') return out.sort((a, b) => a.name.localeCompare(b.name))
  return out.sort(
    (a, b) => (b.bytes ?? -1) - (a.bytes ?? -1) || a.name.localeCompare(b.name),
  )
}

function emptyCounts(): Record<ItemKind, number> {
  return { image: 0, volume: 0, container: 0, buildCache: 0 }
}

export function groupRows(
  rows: StorageItemRow[],
  view: ItemView,
  sort: SortKey,
): ItemGroup[] {
  const map = new Map<string, ItemGroup>()
  for (const r of sortRows(rows, sort)) {
    const key = view === 'type' ? r.kind : (r.ownerApp ?? '')
    let g = map.get(key)
    if (!g) {
      g = {
        key,
        kind: view === 'type' ? r.kind : null,
        app: view === 'app' ? r.ownerApp : null,
        bytes: 0,
        counts: emptyCounts(),
        rows: [],
      }
      map.set(key, g)
    }
    g.rows.push(r)
    g.bytes += r.bytes ?? 0
    g.counts[r.kind] += 1
    g.lastAt = latest(g.lastAt, r.lastAt)
  }
  const groups = [...map.values()]
  if (view === 'type') {
    return groups.sort(
      (a, b) =>
        ITEM_KINDS.indexOf(a.kind ?? 'image') -
        ITEM_KINDS.indexOf(b.kind ?? 'image'),
    )
  }
  // Rows with no app sink to the end whatever their size.
  return groups.sort((a, b) => {
    if (a.app === null) return 1
    if (b.app === null) return -1
    return b.bytes - a.bytes
  })
}
