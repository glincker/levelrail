// Query-key factory and fetcher for GET /api/v1/apps/{name}/logs
// (internal/api/logs.go, TASKS.md 2.3): a historical full-text search
// over already-stored log entries. Distinct from queries/deployLogs.ts,
// which only builds a URL for the live SSE build-log stream and
// deliberately isn't a TanStack Query fetcher at all (see that module's
// own header comment for why a live append-only stream doesn't fit
// Query's freshness model). This is the opposite case: a real request/
// response endpoint over stored data, so it's a normal query, same
// shape as queries/metrics.ts.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { LogEntry, LogsResponse } from '../types/logs'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const logSearchKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'logs'] as const,
  search: (appName: string, fromIso: string, toIso: string, q: string) =>
    [...logSearchKeys.all(appName), fromIso, toIso, q] as const,
}

export interface LogSearchParams {
  from: Date
  to: Date
  /** Full-text search phrase; empty/omitted means every entry in range. */
  q?: string
}

export async function fetchLogEntries(
  appName: string,
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
  return body.entries ?? []
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
    ),
    queryFn: () => fetchLogEntries(appName, params),
  })
}

export function useLogSearch(appName: string, params: LogSearchParams) {
  return useQuery(logSearchQueryOptions(appName, params))
}

// GET /api/v1/apps/{name}/logs/download (internal/api/logs_download.go),
// same from/to/q params as fetchLogEntries above but a plain-text
// attachment, not JSON. Not a TanStack Query fetcher for the same reason
// backupDownloadURL (queries/backupHistory.ts) isn't: the response is a
// raw file stream, consumed as a plain browser navigation target (an
// <a href download>), auth riding along on the same httpOnly session
// cookie every other same-origin request already relies on.
export function logDownloadURL(
  appName: string,
  params: LogSearchParams,
): string {
  const query = new URLSearchParams({
    from: params.from.toISOString(),
    to: params.to.toISOString(),
  })
  if (params.q) {
    query.set('q', params.q)
  }
  return `/api/v1/apps/${encodeURIComponent(appName)}/logs/download?${query.toString()}`
}
