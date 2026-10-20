// Pre-upgrade restore points for Settings > Updates (internal/api/restore_points.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys } from './updates'
import { selfUpgradeKeys } from './selfUpgrade'

export interface RestorePoint {
  id: string
  version: string
  schema: number
  created_at: string
  size_bytes: number
  has_binary: boolean
  has_unit: boolean
  database_restore_needed: boolean
  can_restore: boolean
  cannot_restore_reason?: string
}

export const restorePointKeys = {
  all: [...updatesKeys.all, 'restore-points'] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function restorePointsQueryOptions() {
  return queryOptions({
    queryKey: restorePointKeys.all,
    queryFn: async (): Promise<RestorePoint[]> => {
      const res = await fetch('/api/v1/updates/restore-points')
      if (!res.ok) throw await failure(res, 'fetch restore points')
      const body = (await res.json()) as { restore_points: RestorePoint[] }
      return body.restore_points
    },
    staleTime: 15_000,
    retry: false,
  })
}

export function useRestoreRestorePoint() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: string): Promise<{ status: string }> => {
      const res = await fetch(
        `/api/v1/updates/restore-points/${encodeURIComponent(id)}/restore`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ confirm: id }),
        },
      )
      if (!res.ok) throw await failure(res, 'restore')
      return (await res.json()) as { status: string }
    },
    onSuccess: () =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: restorePointKeys.all }),
        queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.attempts() }),
      ]),
  })
}
