// Live counts for engines with no data browser (search, vector, Memcached),
// GET /api/v1/databases/{name}/engine-stats.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface EngineStatObject {
  name: string
  count: number
}

export interface DatabaseEngineStats {
  engine: string
  objects: EngineStatObject[]
  total: number
  metrics?: Partial<Record<MemcachedMetric, number>>
}

export type MemcachedMetric =
  | 'curr_items'
  | 'bytes'
  | 'limit_maxbytes'
  | 'get_hits'
  | 'get_misses'
  | 'evictions'
  | 'curr_connections'
  | 'uptime'

export const databaseEngineStatsKeys = {
  stats: (name: string) =>
    [...databaseKeys.detail(name), 'engine-stats'] as const,
}

export async function fetchDatabaseEngineStats(
  name: string,
): Promise<DatabaseEngineStats> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/engine-stats`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch engine stats failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseEngineStats
}

const STATS_REFRESH_MS = 15_000

export function databaseEngineStatsQueryOptions(name: string) {
  return queryOptions({
    queryKey: databaseEngineStatsKeys.stats(name),
    queryFn: () => fetchDatabaseEngineStats(name),
    refetchInterval: STATS_REFRESH_MS,
  })
}

export function useDatabaseEngineStats(name: string) {
  return useQuery(databaseEngineStatsQueryOptions(name))
}
