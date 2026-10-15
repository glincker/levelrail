// /api/v1/apps/{name}/access (internal/api/access_log.go).

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AccessRecord } from '../types/edgeAccess'

export interface EdgeAccessFilters {
  minMs: number
  errorsOnly: boolean
  path: string
}

export const edgeAccessKeys = {
  list: (app: string, f: EdgeAccessFilters) => ['edge-access', app, f] as const,
}

export function edgeAccessQueryOptions(app: string, f: EdgeAccessFilters) {
  return queryOptions({
    queryKey: edgeAccessKeys.list(app, f),
    refetchInterval: 5_000,
    queryFn: async () => {
      const q = new URLSearchParams()
      if (f.minMs > 0) q.set('min_ms', String(f.minMs))
      if (f.errorsOnly) q.set('min_status', '500')
      if (f.path !== '') q.set('path', f.path)
      q.set('limit', '200')
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(app)}/access?${q.toString()}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `list requests failed: ${res.status}`),
        )
      }
      return (await res.json()) as AccessRecord[]
    },
  })
}

export function useEdgeAccess(app: string, f: EdgeAccessFilters) {
  return useQuery(edgeAccessQueryOptions(app, f))
}
