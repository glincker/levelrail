import type { AppListEntry } from '../types/appDetail'
import type { AppListFilters } from './appViews'

export type StatusBucket = 'running' | 'deploying' | 'failing' | 'stopped'
export type StatusFilter = StatusBucket | null
export type ViewMode = 'table' | 'grid'

export const STATUS_BUCKETS: StatusBucket[] = [
  'running',
  'deploying',
  'failing',
  'stopped',
]

export function statusBucket(app: Pick<AppListEntry, 'status'>): StatusBucket {
  const { variant, label } = app.status
  if (variant === 'success') return 'running'
  if (variant === 'destructive') return 'failing'
  // A staged app is idle, not in flight, so it must not pulse as deploying.
  return label === 'Stopped' || label === 'Not deployed'
    ? 'stopped'
    : 'deploying'
}

export function countBuckets(
  apps: Pick<AppListEntry, 'status'>[],
): Record<StatusBucket, number> {
  const counts: Record<StatusBucket, number> = {
    running: 0,
    deploying: 0,
    failing: 0,
    stopped: 0,
  }
  for (const app of apps) counts[statusBucket(app)] += 1
  return counts
}

const VIEW_KEY = 'apps.viewMode.v1'

export function loadViewMode(): ViewMode {
  try {
    return window.localStorage.getItem(VIEW_KEY) === 'grid' ? 'grid' : 'table'
  } catch {
    return 'table'
  }
}

export function storeViewMode(mode: ViewMode): void {
  try {
    window.localStorage.setItem(VIEW_KEY, mode)
  } catch {
    // View mode is a convenience only.
  }
}

export function filterApps(
  apps: AppListEntry[],
  filters: AppListFilters,
  status: StatusFilter,
): AppListEntry[] {
  const q = filters.query.trim().toLowerCase()
  return apps.filter(
    (app) =>
      (status === null || statusBucket(app) === status) &&
      (q === '' ||
        app.name.toLowerCase().includes(q) ||
        app.image.toLowerCase().includes(q) ||
        (app.description ?? '').toLowerCase().includes(q)) &&
      (filters.environments.length === 0 ||
        filters.environments.includes(app.environment_name ?? '')) &&
      (filters.tags.length === 0 ||
        (app.tags ?? []).some((tag) => filters.tags.includes(tag))),
  )
}

export function parseStatusSearch(raw: unknown): StatusFilter {
  return typeof raw === 'string' && (STATUS_BUCKETS as string[]).includes(raw)
    ? (raw as StatusBucket)
    : null
}

export interface AppsSearch {
  status?: StatusBucket
  q?: string
  env?: string
  tags?: string
  labels?: string
}

function csvParam(raw: unknown): string | undefined {
  if (typeof raw !== 'string') return undefined
  const parts = [
    ...new Set(
      raw
        .split(',')
        .map((x) => x.trim())
        .filter(Boolean),
    ),
  ]
  return parts.length > 0 ? parts.join(',') : undefined
}

function textParam(raw: unknown): string | undefined {
  return typeof raw === 'string' && raw.trim() !== '' ? raw : undefined
}

// Every Apps filter lives in the URL so links and saved views restore the whole list state.
export function parseAppsSearch(search: Record<string, unknown>): AppsSearch {
  const out: AppsSearch = {}
  const status = parseStatusSearch(search.status)
  if (status) out.status = status
  const q = textParam(search.q)
  if (q) out.q = q
  const env = csvParam(search.env)
  if (env) out.env = env
  const tags = csvParam(search.tags)
  if (tags) out.tags = tags
  const labels = textParam(search.labels)
  if (labels) out.labels = labels
  return out
}

export function appFiltersFromSearch(s: AppsSearch): AppListFilters {
  return {
    query: s.q ?? '',
    environments: s.env ? s.env.split(',') : [],
    tags: s.tags ? s.tags.split(',') : [],
  }
}

export function appsSearchFrom(
  f: AppListFilters,
  status: StatusFilter,
  labels: string,
): AppsSearch {
  return parseAppsSearch({
    status: status ?? undefined,
    q: f.query,
    env: f.environments.join(','),
    tags: f.tags.join(','),
    labels,
  })
}
