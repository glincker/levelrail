import type {
  CatalogEntry,
  Connection,
  ConnectionHealth,
  ConnectionKind,
  ConnectionScopeType,
} from '../types/connections'

export const CONNECTION_KINDS: readonly ConnectionKind[] = [
  'git',
  'registry',
  'notify',
  'secret',
  'dns',
  'cloud',
  'webhook',
]

export const CONNECTION_SCOPES: readonly ConnectionScopeType[] = [
  'instance',
  'org',
  'app',
]

export const CONNECTION_HEALTHS: readonly ConnectionHealth[] = [
  'ok',
  'degraded',
  'needs_reconnect',
  'expired',
  'unknown',
]

// Existing management page per kind: the hub links out, it never forks them.
export const MANAGE_PATH = {
  git: '/settings/git-connections',
  registry: '/settings/registry-credentials',
  notify: '/settings/notification-channels',
  secret: '/settings/vault',
  dns: '/settings/platform/dns-providers',
  cloud: '/settings/node-providers',
  webhook: '/settings/event-webhooks',
} as const satisfies Record<ConnectionKind, string>

const DAY_MS = 86_400_000

export interface ExpiryInfo {
  /** Whole days until expiry, negative once expired. Null when no expiry is set. */
  days: number | null
  expired: boolean
  autoRenew: boolean
}

// Days, not hours: the tail of the last day still counts as one day left.
export function expiryInfo(conn: Connection, now: number): ExpiryInfo {
  const autoRenew = conn.auth_method === 'oauth'
  if (!conn.expires_at) return { days: null, expired: false, autoRenew }
  const at = new Date(conn.expires_at).getTime()
  if (Number.isNaN(at)) return { days: null, expired: false, autoRenew }
  const diff = at - now
  const days = diff >= 0 ? Math.ceil(diff / DAY_MS) : -Math.ceil(-diff / DAY_MS)
  return { days, expired: diff < 0, autoRenew }
}

// A past expiry beats a stale "ok" the server has not re-evaluated yet.
export function effectiveHealth(
  conn: Connection,
  now: number,
): ConnectionHealth {
  if (conn.health === 'ok' && expiryInfo(conn, now).expired) return 'expired'
  return conn.health
}

// Shape plus text, never colour alone: maps onto the shared status vocabulary.
export function healthState(
  health: ConnectionHealth,
): 'healthy' | 'degraded' | 'failed' | 'expired' | 'pending' {
  switch (health) {
    case 'ok':
      return 'healthy'
    case 'degraded':
      return 'degraded'
    case 'needs_reconnect':
      return 'failed'
    case 'expired':
      return 'expired'
    default:
      return 'pending'
  }
}

// Auto-renewing OAuth tokens near expiry are not the operator's problem.
export function needsAttention(conn: Connection, now: number): boolean {
  const health = effectiveHealth(conn, now)
  if (
    health === 'needs_reconnect' ||
    health === 'expired' ||
    health === 'degraded'
  ) {
    return true
  }
  return conn.expiring_soon && conn.auth_method !== 'oauth'
}

export function attentionList(conns: Connection[], now: number): Connection[] {
  const rank = (c: Connection) => {
    const h = effectiveHealth(c, now)
    if (h === 'expired' || h === 'needs_reconnect') return 0
    if (h === 'degraded') return 1
    return 2
  }
  return conns
    .filter((c) => needsAttention(c, now))
    .sort((a, b) => {
      const byRank = rank(a) - rank(b)
      if (byRank !== 0) return byRank
      const da = expiryInfo(a, now).days ?? Number.MAX_SAFE_INTEGER
      const db = expiryInfo(b, now).days ?? Number.MAX_SAFE_INTEGER
      return da - db
    })
}

export interface ConnectionFilters {
  kind: ConnectionKind | 'all'
  scope: ConnectionScopeType | 'all'
  health: ConnectionHealth | 'all'
  query: string
}

export const DEFAULT_FILTERS: ConnectionFilters = {
  kind: 'all',
  scope: 'all',
  health: 'all',
  query: '',
}

export function filterConnections(
  conns: Connection[],
  filters: ConnectionFilters,
  now: number,
): Connection[] {
  const q = filters.query.trim().toLowerCase()
  return conns.filter((c) => {
    if (filters.kind !== 'all' && c.kind !== filters.kind) return false
    if (filters.scope !== 'all' && c.scope_type !== filters.scope) return false
    if (filters.health !== 'all' && effectiveHealth(c, now) !== filters.health)
      return false
    if (!q) return true
    return [c.label, c.account, c.provider, c.host, c.ref, c.scope_id].some(
      (v) => v?.toLowerCase().includes(q),
    )
  })
}

export function groupByKind(
  conns: Connection[],
): { kind: ConnectionKind; items: Connection[] }[] {
  return CONNECTION_KINDS.map((kind) => ({
    kind,
    items: conns.filter((c) => c.kind === kind),
  })).filter((g) => g.items.length > 0)
}

export function groupCatalog(
  entries: CatalogEntry[],
): { category: string; items: CatalogEntry[] }[] {
  const order: string[] = []
  const by = new Map<string, CatalogEntry[]>()
  for (const e of entries) {
    if (!by.has(e.category)) {
      by.set(e.category, [])
      order.push(e.category)
    }
    by.get(e.category)?.push(e)
  }
  return order.map((category) => ({ category, items: by.get(category) ?? [] }))
}

export function isConnectionKind(value: string): value is ConnectionKind {
  return (CONNECTION_KINDS as readonly string[]).includes(value)
}

// Categories that are not a connection kind fall back to the settings hub.
export function catalogDestination(
  category: string,
): (typeof MANAGE_PATH)[ConnectionKind] | '/settings' {
  return isConnectionKind(category) ? MANAGE_PATH[category] : '/settings'
}
