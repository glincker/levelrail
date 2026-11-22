// Starts a restore test of a database's newest backup
// (POST /api/v1/databases/{name}/restore-test, internal/api/database_restore_check.go).
// The result is read back from the drill history through useBackupDrills.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { backupProtectionKeys } from './backupProtection'

export function useStartDatabaseRestoreTest(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<{ id: string; backup_id: string }, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(
        `/api/v1/databases/${encodeURIComponent(databaseName)}/restore-test`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `restore test failed: ${res.status}`),
        )
      }
      return (await res.json()) as { id: string; backup_id: string }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupProtectionKeys.all })
    },
  })
}
