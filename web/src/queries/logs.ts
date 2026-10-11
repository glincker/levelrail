// Query-key factory and fetcher for GET /api/v1/apps/{name}/logs
// (internal/api/logs.go): a historical full-text search over already-stored
// log entries. A normal request/response query, unlike the live SSE tail.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { LogsResponse, LogsResult } from '../types/logs'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import {
  buildLogQuery,
  formatFieldFilter,
  type FieldFilter,
} from '../lib/logFilters'

export const logSearchKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'logs'] as const,
  search: (
    appName: string,
    fromIso: string,
    toIso: string,
    q: string,
    limit: number,
    level: string,
    container: string,
    stream: string,
    fields: string[],
  ) =>
    [
      ...logSearchKeys.all(appName),
      fromIso,
      toIso,
      q,
      limit,
      level,
      container,
      stream,
      fields,
    ] as const,
}

export interface LogSearchParams {
  from: Date
  to: Date
  /** Full-text search phrase; empty/omitted means every entry in range. */
  q?: string
  /** Keep only the newest N matches; the response total still counts all. */
  limit?: number
  /** Minimum level (trace, debug, info, warn, error, fatal). */
  level?: string
  /** Container id prefix. */
  container?: string
  stream?: 'stdout' | 'stderr'
  fields?: readonly FieldFilter[]
}

export async function fetchLogEntries(
  appName: string,
  params: LogSearchParams,
): Promise<LogsResult> {
  const query = buildLogQuery(params)
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/logs?${query.toString()}`,
  )
  if (res.status === 501) {
    throw new ApiError(501, 'telemetry is not configured on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch logs failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as LogsResponse
  const entries = body.entries ?? []
  return {
    entries,
    total: body.total ?? entries.length,
    containers: body.containers ?? [],
  }
}

export function logSearchQueryOptions(
  appName: string,
  params: LogSearchParams,
) {
  return queryOptions({
    queryKey: logSearchKeys.search(
      appName,
      params.from.toISOString(),
      params.to.toISOString(),
      params.q ?? '',
      params.limit ?? 0,
      params.level ?? '',
      params.container ?? '',
      params.stream ?? '',
      (params.fields ?? []).map(formatFieldFilter),
    ),
    queryFn: () => fetchLogEntries(appName, params),
  })
}

export function useLogSearch(appName: string, params: LogSearchParams) {
  return useQuery(logSearchQueryOptions(appName, params))
}

// Same filters as fetchLogEntries but a plain-text attachment, consumed as
// a browser navigation target (<a href download>), not a Query fetcher.
export function logDownloadURL(
  appName: string,
  params: LogSearchParams,
): string {
  const query = buildLogQuery({ ...params, limit: undefined })
  return `/api/v1/apps/${encodeURIComponent(appName)}/logs/download?${query.toString()}`
}
