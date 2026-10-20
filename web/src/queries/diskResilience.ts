import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DiskLevel = 'unknown' | 'ok' | 'low' | 'critical'

export interface DiskVolume {
  name: string
  path: string
  free_bytes: number
  total_bytes: number
  level: DiskLevel
}

// Matches internal/api's diskResilienceResponse.
export interface DiskResilience {
  volumes: DiskVolume[]
  thresholds: {
    low_free_bytes: number
    critical_free_bytes: number
    reserve_bytes: number
  }
  worst: DiskLevel
  reserve_held_bytes: number
  refusing: boolean
  free_space_available: boolean
}

export interface FreeSpaceResult {
  reclaimed_bytes: number
  disk: DiskResilience
}

const KEY = ['disk-resilience']
const URL = '/api/v1/resilience/disk'

export async function fetchDiskResilience(
  signal?: AbortSignal,
): Promise<DiskResilience> {
  const res = await fetch(URL, { signal, cache: 'no-store' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch disk status failed: ${res.status}`),
    )
  }
  return (await res.json()) as DiskResilience
}

export function useDiskResilience() {
  return useQuery({
    queryKey: KEY,
    queryFn: ({ signal }) => fetchDiskResilience(signal),
    staleTime: 30_000,
  })
}

export function useFreeDiskSpace() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<FreeSpaceResult> => {
      const res = await fetch(`${URL}/free-space`, { method: 'POST' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `free space failed: ${res.status}`),
        )
      }
      return (await res.json()) as FreeSpaceResult
    },
    onSuccess: (r) => qc.setQueryData(KEY, r.disk),
  })
}
