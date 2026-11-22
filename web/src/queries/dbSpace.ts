import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DBSpacePart {
  id: string
  bytes: number
  tables: number
  /** Cleanup center targets that prune this part. */
  cleanup_targets?: string[]
  /** Control plane health fix that prunes this part when no cleanup target does. */
  fix?: string
}

export interface DBSpace {
  total_bytes: number
  reclaimable_bytes: number
  parts: DBSpacePart[] | null
}

export const dbSpaceKey = ['system', 'db-space'] as const

export function useDBSpace() {
  return useQuery({
    queryKey: dbSpaceKey,
    queryFn: async (): Promise<DBSpace> => {
      const res = await fetch('/api/v1/system/db-space')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `load storage use failed: ${res.status}`),
        )
      }
      return (await res.json()) as DBSpace
    },
    staleTime: 30_000,
  })
}
