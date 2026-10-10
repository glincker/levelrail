// Read-only view of GET /api/v1/imports/platform/databases/{name}: the
// copy status of a database that was created as a migration target. Any
// failure is treated as "no copy info", so the explorer never breaks on it.

import { queryOptions, useQuery } from '@tanstack/react-query'

export type DataCopyStatus =
  'pending' | 'copying' | 'verified' | 'failed' | 'unsupported'

export interface DataCopyInfo {
  database: string
  status: DataCopyStatus
  reason?: string
  checked: number
  mismatched: number
  finished_at?: string
}

async function fetchDataCopy(name: string): Promise<DataCopyInfo | null> {
  try {
    const res = await fetch(
      `/api/v1/imports/platform/databases/${encodeURIComponent(name)}`,
    )
    if (!res.ok) return null
    return (await res.json()) as DataCopyInfo
  } catch {
    return null
  }
}

export function useDatabaseDataCopy(name: string) {
  return useQuery(
    queryOptions({
      queryKey: ['databases', 'detail', name, 'data-copy'] as const,
      queryFn: () => fetchDataCopy(name),
      retry: false,
      staleTime: 30_000,
    }),
  )
}
