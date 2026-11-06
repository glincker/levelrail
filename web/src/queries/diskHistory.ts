// GET /api/v1/system/disk/history (internal/api/disk_history.go). 501 means no
// sample store on this control plane, a real state the panel renders calmly.

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { DEDUPE_STALE_MS } from '../lib/queryStale'

export interface DiskSample {
  at: string
  free_bytes: number
  total_bytes: number
  images_bytes: number
  containers_bytes: number
  volumes_bytes: number
  build_cache_bytes: number
}

export interface DiskHistory {
  samples: DiskSample[]
  forecast: { days_until_full: number | null; basis_days: number }
  retention_days: number
}

export const diskHistoryKeys = {
  all: ['system', 'disk', 'history'] as const,
  days: (days: number) => ['system', 'disk', 'history', days] as const,
}

export async function fetchDiskHistory(days: number): Promise<DiskHistory> {
  const res = await fetch(`/api/v1/system/disk/history?days=${days}`)
  if (res.status === 501) {
    throw new ApiError(501, 'disk history is not stored on this control plane')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch disk history failed: ${res.status}`),
    )
  }
  return (await res.json()) as DiskHistory
}

// retry: false because a 501 never succeeds on retry; previous data keeps the chart steady on range switch.
export function useDiskHistory(days: number) {
  return useQuery({
    queryKey: diskHistoryKeys.days(days),
    queryFn: () => fetchDiskHistory(days),
    staleTime: DEDUPE_STALE_MS,
    retry: false,
    placeholderData: keepPreviousData,
  })
}
