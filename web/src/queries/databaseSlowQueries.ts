// Query-key factory and fetcher for GET
// /api/v1/databases/{name}/slow-queries
// (internal/api/database_slow_queries.go): parsed slow query log entries
// for a Postgres/MySQL database, sorted by duration descending. Same
// query-key/fetch shape as queries/databaseLogs.ts, only the endpoint and
// response type differ.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { SlowQueryEntry, SlowQueriesResponse } from '../types/slowQueries'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface SlowQueryParams {
  from: Date
  to: Date
  limit?: number
  offset?: number
}

export interface SlowQueryResult {
  entries: SlowQueryEntry[]
  total: number
}

export const databaseSlowQueryKeys = {
  all: (databaseName: string) =>
    [...databaseKeys.detail(databaseName), 'slow-queries'] as const,
  list: (
    databaseName: string,
    fromIso: string,
    toIso: string,
    limit: number | undefined,
    offset: number | undefined,
  ) =>
    [
      ...databaseSlowQueryKeys.all(databaseName),
      fromIso,
      toIso,
      limit ?? null,
      offset ?? null,
    ] as const,
}

export async function fetchDatabaseSlowQueries(
  databaseName: string,
  params: SlowQueryParams,
): Promise<SlowQueryResult> {
  const query = new URLSearchParams({
    from: params.from.toISOString(),
    to: params.to.toISOString(),
  })
  if (params.limit !== undefined) {
    query.set('limit', String(params.limit))
  }
  if (params.offset !== undefined) {
    query.set('offset', String(params.offset))
  }
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/slow-queries?${query.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (res.status === 400) {
    throw new ApiError(
      400,
      await readErrorMessage(
        res,
        'slow query log is not supported for this database engine',
      ),
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch slow queries failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as SlowQueriesResponse
  return { entries: body.entries ?? [], total: body.total ?? 0 }
}

export function databaseSlowQueriesQueryOptions(
  databaseName: string,
  params: SlowQueryParams,
) {
  return queryOptions({
    queryKey: databaseSlowQueryKeys.list(
      databaseName,
      params.from.toISOString(),
      params.to.toISOString(),
      params.limit,
      params.offset,
    ),
    queryFn: () => fetchDatabaseSlowQueries(databaseName, params),
  })
}

export function useDatabaseSlowQueries(
  databaseName: string,
  params: SlowQueryParams,
) {
  return useQuery(databaseSlowQueriesQueryOptions(databaseName, params))
}
