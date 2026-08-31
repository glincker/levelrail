// Query-key factory and fetchers for the backup verification
// sub-resource of one app service volume backup attempt:
// POST/GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify(ications)
// (internal/api/app_volume_backup_verify.go). Mirrors
// queries/backupVerification.ts's exact shape for the database resource
// kind.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { BackupVerificationRecord } from '../types/backupVerification'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const volumeBackupVerificationKeys = {
  all: (appName: string, volumeName: string, backupHistoryId: string) =>
    [
      'apps',
      appName,
      'volumes',
      volumeName,
      'backups',
      backupHistoryId,
      'verifications',
    ] as const,
  list: (appName: string, volumeName: string, backupHistoryId: string) =>
    [
      ...volumeBackupVerificationKeys.all(appName, volumeName, backupHistoryId),
      'list',
    ] as const,
}

const RUNNING_POLL_INTERVAL_MS = 3_000

function volumeBackupVerifyPath(
  appName: string,
  volumeName: string,
  backupHistoryId: string,
): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/backups/${encodeURIComponent(backupHistoryId)}`
}

export async function fetchVolumeBackupVerifications(
  appName: string,
  volumeName: string,
  backupHistoryId: string,
): Promise<BackupVerificationRecord[]> {
  const res = await fetch(
    `${volumeBackupVerifyPath(appName, volumeName, backupHistoryId)}/verifications`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch volume backup verifications failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as BackupVerificationRecord[] | null
  return body ?? []
}

export function useLatestVolumeBackupVerification(
  appName: string,
  volumeName: string,
  backupHistoryId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: volumeBackupVerificationKeys.list(
      appName,
      volumeName,
      backupHistoryId,
    ),
    queryFn: () =>
      fetchVolumeBackupVerifications(appName, volumeName, backupHistoryId),
    enabled,
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

export async function verifyVolumeBackup(
  appName: string,
  volumeName: string,
  backupHistoryId: string,
): Promise<BackupVerificationRecord> {
  const res = await fetch(
    `${volumeBackupVerifyPath(appName, volumeName, backupHistoryId)}/verify`,
    { method: 'POST' },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Backup verification requires a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `verify volume backup failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupVerificationRecord
}

export function useVerifyVolumeBackup(
  appName: string,
  volumeName: string,
  backupHistoryId: string,
) {
  const queryClient = useQueryClient()
  return useMutation<BackupVerificationRecord, ApiError, void>({
    mutationFn: () => verifyVolumeBackup(appName, volumeName, backupHistoryId),
    onSuccess: (record) => {
      queryClient.setQueryData(
        volumeBackupVerificationKeys.list(appName, volumeName, backupHistoryId),
        (existing: BackupVerificationRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: volumeBackupVerificationKeys.list(
          appName,
          volumeName,
          backupHistoryId,
        ),
      })
    },
  })
}
