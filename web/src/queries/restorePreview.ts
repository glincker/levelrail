// GET /api/v1/databases/{name}/restore-preview (handleDatabaseRestorePreview):
// read-only size, age, target and overwrite scope of one restore.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface RestorePreview {
  database: string
  backup_id: string
  engine: string
  size_bytes: number
  taken_at: string
  age_seconds: number
  target_id: string
  target_name?: string
  encrypted: boolean
  overwrites: string
  safety_backup_id?: string
  safety_taken_at?: string
  busy: boolean
}

export async function fetchRestorePreview(
  databaseName: string,
  backupId: string,
): Promise<RestorePreview> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/restore-preview?backup_id=${encodeURIComponent(backupId)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `restore preview failed: ${res.status}`),
    )
  }
  return (await res.json()) as RestorePreview
}

export function useRestorePreview(
  databaseName: string,
  backupId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: ['databases', databaseName, 'restore-preview', backupId],
    queryFn: () => fetchRestorePreview(databaseName, backupId),
    enabled,
    staleTime: 0,
  })
}
