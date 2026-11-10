export type SavedViewList =
  'apps' | 'databases' | 'domains' | 'builds' | 'deployments'
export type ViewFilter = Record<string, string>

export interface SavedView {
  id: string
  name: string
  list: SavedViewList
  filter: ViewFilter
  shared: 'private' | 'team'
  mine: boolean
  can_delete: boolean
}

// Mirrors the server allowlist: a stored filter can only ever set these URL params of its own list.
const VIEW_KEYS: Record<SavedViewList, readonly string[]> = {
  apps: ['q', 'status', 'env', 'tags', 'labels'],
  databases: ['q', 'status', 'engine', 'tag', 'labels'],
  domains: ['q', 'status'],
  builds: ['q', 'status', 'app'],
  deployments: ['q', 'status', 'app', 'env'],
}

export function sanitizeViewFilter(
  list: SavedViewList,
  filter: unknown,
): ViewFilter {
  const out: ViewFilter = {}
  if (typeof filter !== 'object' || filter === null) return out
  const rec = filter as Record<string, unknown>
  for (const key of VIEW_KEYS[list]) {
    const v = Object.prototype.hasOwnProperty.call(rec, key)
      ? rec[key]
      : undefined
    if (typeof v === 'string' && v.trim() !== '') out[key] = v
  }
  return out
}
