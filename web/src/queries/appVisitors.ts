// GET /api/v1/apps/{name}/visitors: per-app visitor summary from the edge
// access log ring. 501 means the access log is not wired on this install.

import { keepPreviousData, queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { VISITORS_REFETCH_MS } from '../components/overview/config'

export type VisitorsWindow = '1h' | '24h' | '7d' | '30d'

export interface VisitorCount {
  label: string
  count: number
}

export interface VisitorBucket {
  start: string
  requests: number
  visitors: number
}

export interface VisitorDomain {
  domain: string
  requests: number
  visitors: number
}

export interface AppVisitors {
  app: string
  domain?: string
  window: VisitorsWindow
  since: string
  generated_at: string
  requests: number
  visitors: number
  status: Record<'2xx' | '3xx' | '4xx' | '5xx', number>
  top_paths: VisitorCount[]
  top_referrers: VisitorCount[]
  series: VisitorBucket[]
  domains: VisitorDomain[]
  oldest_record?: string
  salt_period: string
}

export const visitorsKey = (app: string, window: VisitorsWindow, domain = '') =>
  ['apps', app, 'visitors', window, domain] as const

export function isVisitorsUnavailable(err: unknown): boolean {
  return err instanceof ApiError && err.status === 501
}

export function appVisitorsQueryOptions(
  app: string,
  window: VisitorsWindow,
  domain = '',
) {
  return queryOptions({
    queryKey: visitorsKey(app, window, domain),
    queryFn: async (): Promise<AppVisitors> => {
      const qs = new URLSearchParams({ window })
      if (domain) qs.set('domain', domain)
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/visitors?${qs.toString()}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `load visitors failed: ${res.status}`),
        )
      }
      return (await res.json()) as AppVisitors
    },
    staleTime: VISITORS_REFETCH_MS / 2,
    refetchInterval: VISITORS_REFETCH_MS,
    placeholderData: keepPreviousData,
    retry: (count, err) => !isVisitorsUnavailable(err) && count < 2,
  })
}

export function useAppVisitors(
  app: string,
  window: VisitorsWindow,
  domain = '',
) {
  return useQuery(appVisitorsQueryOptions(app, window, domain))
}
