// Undo window for deleted apps and databases (GET and POST /api/v1/recently-deleted).
import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'
import { useApiMutation } from './useApiMutation'

export type RecentlyDeletedKind = 'app' | 'database'

export interface RecentlyDeletedEntry {
  kind: RecentlyDeletedKind
  name: string
  deleted_at: string
  expires_at: string
  deleted_by?: string
  last_backup?: { id: string; target_id: string; object_key: string }
}

export const recentlyDeletedKeys = {
  all: ['recently-deleted'] as const,
}

async function fetchRecentlyDeleted(): Promise<RecentlyDeletedEntry[]> {
  const res = await fetch('/api/v1/recently-deleted')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `list recently deleted failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RecentlyDeletedEntry[]
}

export function useRecentlyDeleted() {
  return useQuery({
    queryKey: recentlyDeletedKeys.all,
    queryFn: fetchRecentlyDeleted,
  })
}

export function useRestoreRecentlyDeleted() {
  return useApiMutation({
    fn: async (entry: Pick<RecentlyDeletedEntry, 'kind' | 'name'>) => {
      const res = await fetch(
        `/api/v1/recently-deleted/${entry.kind}/${encodeURIComponent(entry.name)}/restore`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `restore failed: ${res.status}`),
        )
      }
    },
    invalidate: [recentlyDeletedKeys.all, appKeys.all, databaseKeys.all],
  })
}
