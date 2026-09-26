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
  return label === 'Stopped' ? 'stopped' : 'deploying'
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
        app.image.toLowerCase().includes(q)) &&
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
