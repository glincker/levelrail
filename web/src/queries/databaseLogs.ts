// Query-key factory and fetcher for GET /api/v1/databases/{name}/logs
// (internal/api/database_logs.go): the database-kind counterpart to
// queries/logs.ts. Same wire shape (LogEntry/LogsResponse, types/logs.ts)
// and same historical full-text-search semantics, reusing that type file
// directly since the response shape is identical, only the resource kind
// the resourceID resolves to differs.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { LogEntry, LogsResponse } from '../types/logs'
import { databaseKeys } from './databases'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { LogSearchParams } from './logs'

export const databaseLogSearchKeys = {
  all: (databaseName: string) =>
    [...databaseKeys.detail(databaseName), 'logs'] as const,
  search: (databaseName: string, fromIso: string, toIso: string, q: string) =>
    [...databaseLogSearchKeys.all(databaseName), fromIso, toIso, q] as const,
}

export async function fetchDatabaseLogEntries(
  databaseName: string,
  params: LogSearchParams,
): Promise<LogEntry[]> {
  const query = new URLSearchParams({
    from: params.from.toISOString(),
    to: params.to.toISOString(),
  })
  if (params.q) {
    query.set('q', params.q)
  }
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/logs?${query.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch database logs failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as LogsResponse
  return body.entries ?? []
}

export function databaseLogSearchQueryOptions(
  databaseName: string,
  params: LogSearchParams,
) {
  return queryOptions({
    queryKey: databaseLogSearchKeys.search(
      databaseName,
      params.from.toISOString(),
      params.to.toISOString(),
      params.q ?? '',
    ),
    queryFn: () => fetchDatabaseLogEntries(databaseName, params),
  })
}

export function useDatabaseLogSearch(
  databaseName: string,
  params: LogSearchParams,
) {
  return useQuery(databaseLogSearchQueryOptions(databaseName, params))
}
