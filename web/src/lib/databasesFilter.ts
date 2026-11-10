import type { DatabaseListEntry } from '../types/databaseDetail'

export const DB_STATUS_BUCKETS = [
  'healthy',
  'attention',
  'reconciling',
  'stopped',
] as const
export type DbStatusBucket = (typeof DB_STATUS_BUCKETS)[number]

export interface DatabaseListFilters {
  q: string
  engines: string[]
  status: DbStatusBucket | null
  tags: string[]
}

export const EMPTY_DB_FILTERS: DatabaseListFilters = {
  q: '',
  engines: [],
  status: null,
  tags: [],
}

// URL shape: engines travel as one comma list so links stay short.
export interface DatabasesSearch {
  create?: string
  q?: string
  engine?: string
  status?: DbStatusBucket
  tag?: string
}

// Buckets come from fields the list endpoint already sends: the
// suspended flag, provision_state and the server status summary.
export function databaseStatusBucket(db: DatabaseListEntry): DbStatusBucket {
  if (db.suspended || db.status.label === 'Stopped') return 'stopped'
  if (db.provision_state === 'failed' || db.status.variant === 'destructive') {
    return 'attention'
  }
  if (db.status.variant === 'success') return 'healthy'
  return 'reconciling'
}

export function countByStatus(
  dbs: readonly DatabaseListEntry[],
): Record<DbStatusBucket, number> {
  const out: Record<DbStatusBucket, number> = {
    healthy: 0,
    attention: 0,
    reconciling: 0,
    stopped: 0,
  }
  for (const db of dbs) out[databaseStatusBucket(db)] += 1
  return out
}

export function engineOptions(dbs: readonly DatabaseListEntry[]): string[] {
  return [...new Set(dbs.map((db) => db.engine))].sort()
}

export function tagOptions(dbs: readonly DatabaseListEntry[]): string[] {
  return [...new Set(dbs.flatMap((db) => db.tags ?? []))].sort()
}

// A row matches when it carries every selected tag.
function hasTags(db: DatabaseListEntry, want: readonly string[]): boolean {
  const have = db.tags ?? []
  return want.every((tag) => have.includes(tag))
}

function splitList(raw: string): string[] {
  return raw
    .split(',')
    .map((e) => e.trim())
    .filter(Boolean)
}

export function filterDatabases(
  dbs: readonly DatabaseListEntry[],
  f: DatabaseListFilters,
): DatabaseListEntry[] {
  const q = f.q.trim().toLowerCase()
  return dbs.filter(
    (db) =>
      (q === '' ||
        db.name.toLowerCase().includes(q) ||
        db.engine.toLowerCase().includes(q)) &&
      (f.engines.length === 0 || f.engines.includes(db.engine)) &&
      (f.status === null || databaseStatusBucket(db) === f.status) &&
      hasTags(db, f.tags),
  )
}

export function dbFiltersActive(f: DatabaseListFilters): boolean {
  return (
    f.q !== '' || f.engines.length > 0 || f.status !== null || f.tags.length > 0
  )
}

function isBucket(raw: unknown): raw is DbStatusBucket {
  return (
    typeof raw === 'string' &&
    (DB_STATUS_BUCKETS as readonly string[]).includes(raw)
  )
}

export function parseDatabasesSearch(
  search: Record<string, unknown>,
): DatabasesSearch {
  const out: DatabasesSearch = {}
  if (search.create === '1' || search.create === 1) out.create = '1'
  if (typeof search.q === 'string' && search.q !== '') out.q = search.q
  if (typeof search.engine === 'string') {
    const engines = search.engine
      .split(',')
      .map((e) => e.trim())
      .filter(Boolean)
    if (engines.length > 0) out.engine = engines.join(',')
  }
  if (isBucket(search.status)) out.status = search.status
  if (typeof search.tag === 'string') {
    const tags = splitList(search.tag.toLowerCase())
    if (tags.length > 0) out.tag = tags.join(',')
  }
  return out
}

export function filtersFromSearch(s: DatabasesSearch): DatabaseListFilters {
  return {
    q: s.q ?? '',
    engines: s.engine ? s.engine.split(',') : [],
    status: s.status ?? null,
    tags: s.tag ? s.tag.split(',') : [],
  }
}

// The saved views filter for the Databases list, keyed like its URL params.
export function databaseViewFilter(
  f: DatabaseListFilters,
  labels: string,
): Record<string, string> {
  return {
    q: f.q,
    status: f.status ?? '',
    engine: f.engines.join(','),
    tag: f.tags.join(','),
    labels,
  }
}

export function searchFromFilters(f: DatabaseListFilters): DatabasesSearch {
  const out: DatabasesSearch = {}
  if (f.q !== '') out.q = f.q
  if (f.engines.length > 0) out.engine = f.engines.join(',')
  if (f.status) out.status = f.status
  if (f.tags.length > 0) out.tag = f.tags.join(',')
  return out
}

export type DbBulkAction = 'start' | 'stop' | 'backup'
export type BulkItemStatus =
  'planned' | 'ok' | 'skipped' | 'denied' | 'protected' | 'notFound' | 'error'
export type SkipReason =
  'external' | 'notCreated' | 'running' | 'stopped' | 'noTarget' | 'noBackups'

export interface BulkItemResult {
  name: string
  status: BulkItemStatus
  reason?: string
}

// Skip reasons are known before any call, so the dialog can show them up front.
export function planBulk(
  action: DbBulkAction,
  dbs: readonly DatabaseListEntry[],
): BulkItemResult[] {
  return dbs.map((db): BulkItemResult => {
    const skip = (reason: SkipReason): BulkItemResult => ({
      name: db.name,
      status: 'skipped',
      reason,
    })
    if (db.external) return skip('external')
    if (db.provision_state) return skip('notCreated')
    if (action === 'start' && !db.suspended) return skip('running')
    if (action === 'stop' && db.suspended) return skip('stopped')
    if (action === 'backup' && !db.backup_target_id) return skip('noTarget')
    return { name: db.name, status: 'planned' }
  })
}
