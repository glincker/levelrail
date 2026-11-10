// GET /api/v1/apps/{name}/access/search (internal/api/access_log_search.go).

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AccessRecord } from '../types/edgeAccess'

export interface AccessSearchFilters {
  status: string
  method: string
  path: string
  q: string
  since: string
  cursor: string
}

export interface AccessSearchRow extends AccessRecord {
  app?: string
}

export interface AccessSearchPage {
  records: AccessSearchRow[]
  next_cursor?: string
  truncated: boolean
  from: string
  to: string
  max_rows: number
  max_window_seconds: number
}

export function accessSearchParams(f: AccessSearchFilters): URLSearchParams {
  const q = new URLSearchParams()
  if (f.status !== '') q.set('status', f.status)
  if (f.method !== '') q.set('method', f.method)
  if (f.path !== '') q.set('path', f.path)
  if (f.q !== '') q.set('q', f.q)
  if (f.since !== '') q.set('since', f.since)
  if (f.cursor !== '') q.set('cursor', f.cursor)
  return q
}

export function accessSearchQueryOptions(app: string, f: AccessSearchFilters) {
  return queryOptions({
    queryKey: ['access-search', app, f] as const,
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/access/search?${accessSearchParams(f).toString()}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `search requests failed: ${res.status}`),
        )
      }
      return (await res.json()) as AccessSearchPage
    },
  })
}

export function useAccessSearch(app: string, f: AccessSearchFilters) {
  return useQuery(accessSearchQueryOptions(app, f))
}
