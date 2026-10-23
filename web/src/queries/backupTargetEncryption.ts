// PUT /api/v1/backup-targets/{id}/encryption (handleSetBackupTargetEncryption, root only).
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { backupTargetKeys } from './backupTargets'
import type { BackupTargetEncryption } from '../types/backupTarget'

export async function setBackupTargetEncryption(
  id: string,
  enabled: boolean,
): Promise<BackupTargetEncryption> {
  const res = await fetch(
    `/api/v1/backup-targets/${encodeURIComponent(id)}/encryption`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update encryption failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupTargetEncryption
}

export function useSetBackupTargetEncryption() {
  const queryClient = useQueryClient()
  return useMutation<
    BackupTargetEncryption,
    ApiError,
    { id: string; enabled: boolean }
  >({
    mutationFn: ({ id, enabled }) => setBackupTargetEncryption(id, enabled),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupTargetKeys.list() })
    },
  })
}
