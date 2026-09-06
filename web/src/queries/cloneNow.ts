// Fetcher and mutation for POST /api/v1/databases/{name}/clone
// (internal/api/database_clone_now.go): mirrors queries/cloneRestore.ts's
// own shape, since this writes into the exact same clone_restores (and
// backup_history) rows that action does, just via a different trigger
// endpoint. On success, both of those lists (and the database list, for
// the brand-new database itself) get invalidated the same way
// useTriggerCloneRestore already does.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import type { CloneNowRequest, CloneNowResult } from '../types/cloneNow'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'
import { cloneRestoreKeys } from './cloneRestore'
import { backupHistoryKeys } from './backupHistory'

export async function cloneDatabaseNow(
  sourceDatabaseName: string,
  req: CloneNowRequest,
): Promise<CloneNowResult> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(sourceDatabaseName)}/clone`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Cloning requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clone database failed: ${res.status}`),
    )
  }
  return (await res.json()) as CloneNowResult
}

export function useCloneDatabaseNow(sourceDatabaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<CloneNowResult, ApiError, CloneNowRequest>({
    mutationFn: (req: CloneNowRequest) =>
      cloneDatabaseNow(sourceDatabaseName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: cloneRestoreKeys.list(sourceDatabaseName),
      })
      void queryClient.invalidateQueries({
        queryKey: backupHistoryKeys.list(sourceDatabaseName),
      })
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}
