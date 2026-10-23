// Server-paged app list and the fleet summary, so the dashboard and the apps
// page never download every app at once (GET /api/v1/apps with limit/offset
// and X-Total-Count, GET /api/v1/apps-summary for the counts).

import { infiniteQueryOptions, queryOptions } from '@tanstack/react-query'
import type { AppListEntry } from '../types/appDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { getEnvironmentScope } from '../lib/environmentScope'
import { getProjectScope } from '../lib/projectScope'
import { DEDUPE_STALE_MS } from '../lib/queryStale'
import type { StatusBucket } from '../lib/appsListView'
import { appKeys } from './apps'

export const APP_PAGE_SIZE = 100
export const APP_SAMPLE_SIZE = 8

export interface AppsSummary {
  total: number
  running: number
  failing: number
  deploying: number
  stopped: number
  attention: number
  by_environment: Record<string, number>
}

export interface AppPageParams {
  q?: string
  environments?: readonly string[]
  tags?: readonly string[]
  status?: StatusBucket | null
  limit?: number
}

export interface AppPage {
  items: AppListEntry[]
  total: number
  offset: number
}

function pageQuery(params: AppPageParams, offset: number): string {
  const qs = new URLSearchParams()
  const q = params.q?.trim()
  if (q) qs.set('q', q)
  const envs = params.environments?.length
    ? params.environments
    : [getEnvironmentScope()].filter((e) => e !== '')
  for (const env of envs) qs.append('environment', env)
  const project = getProjectScope()
  if (project) qs.set('project', project)
  if (params.tags?.length) {
    qs.set('tag', params.tags.join(','))
    qs.set('tag_match', 'any')
  }
  if (params.status) qs.set('status', params.status)
  qs.set('limit', String(params.limit ?? APP_PAGE_SIZE))
  if (offset > 0) qs.set('offset', String(offset))
  return qs.toString()
}

export async function fetchAppPage(
  params: AppPageParams,
  offset = 0,
  signal?: AbortSignal,
): Promise<AppPage> {
  const res = await fetch(`/api/v1/apps?${pageQuery(params, offset)}`, {
    signal,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch apps failed: ${String(res.status)}`),
    )
  }
  const items = (await res.json()) as AppListEntry[]
  const total = Number(res.headers.get('X-Total-Count') ?? items.length)
  return { items, total, offset }
}

export function appPagesQueryOptions(params: AppPageParams) {
  return infiniteQueryOptions({
    queryKey: [
      ...appKeys.all,
      'pages',
      params,
      getEnvironmentScope(),
      getProjectScope(),
    ] as const,
    queryFn: ({ pageParam, signal }) => fetchAppPage(params, pageParam, signal),
    initialPageParam: 0,
    getNextPageParam: (last) => {
      const next = last.offset + last.items.length
      return last.items.length > 0 && next < last.total ? next : undefined
    },
    staleTime: DEDUPE_STALE_MS,
  })
}

export function appsSummaryQueryOptions() {
  const environment = getEnvironmentScope()
  const project = getProjectScope()
  return queryOptions({
    queryKey: [...appKeys.all, 'summary', environment, project] as const,
    queryFn: async ({ signal }): Promise<AppsSummary> => {
      const params = new URLSearchParams()
      if (environment) params.set('environment', environment)
      if (project) params.set('project', project)
      const qs = params.size ? `?${params.toString()}` : ''
      const res = await fetch(`/api/v1/apps-summary${qs}`, { signal })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch apps summary failed: ${String(res.status)}`,
          ),
        )
      }
      return (await res.json()) as AppsSummary
    },
    staleTime: DEDUPE_STALE_MS,
  })
}

// Unscoped on purpose: a project or environment filter with no apps must not
// look like a fresh install.
export function anyAppsQueryOptions() {
  return queryOptions({
    queryKey: [...appKeys.all, 'any'] as const,
    queryFn: async ({ signal }): Promise<boolean> => {
      const res = await fetch('/api/v1/apps?limit=1', { signal })
      if (!res.ok) return false
      const items = (await res.json()) as unknown[]
      return Number(res.headers.get('X-Total-Count') ?? items.length) > 0
    },
    staleTime: DEDUPE_STALE_MS,
  })
}

// Failing apps first, then the first page by name: what the dashboard needs
// to sample deploys and traffic without the whole list.
export function appSampleQueryOptions() {
  return queryOptions({
    queryKey: [
      ...appKeys.all,
      'sample',
      getEnvironmentScope(),
      getProjectScope(),
    ] as const,
    queryFn: async (): Promise<AppListEntry[]> => {
      const limit = APP_SAMPLE_SIZE
      const [failing, first] = await Promise.all([
        fetchAppPage({ status: 'failing', limit }),
        fetchAppPage({ limit }),
      ])
      const seen = new Set<string>()
      return [...failing.items, ...first.items].filter((a) => {
        if (seen.has(a.name)) return false
        seen.add(a.name)
        return true
      })
    },
    staleTime: DEDUPE_STALE_MS,
  })
}

// Every failing app, for the attention feed. Bounded by the page cap.
export function failingAppsQueryOptions() {
  return queryOptions({
    queryKey: [
      ...appKeys.all,
      'failing',
      getEnvironmentScope(),
      getProjectScope(),
    ] as const,
    queryFn: async () =>
      (await fetchAppPage({ status: 'failing', limit: APP_PAGE_SIZE })).items,
    staleTime: DEDUPE_STALE_MS,
  })
}
