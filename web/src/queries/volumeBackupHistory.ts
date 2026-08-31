// Query-key factory and fetchers for the backup history sub-resource of
// an app service's named volume:
// GET/POST /api/v1/apps/{name}/volumes/{volume}/backups
// (internal/api/app_volume_backups.go). Mirrors queries/backupHistory.ts's
// exact shape for the database resource kind, kept as its own module
// rather than generalizing that one over a "database or volume" scope
// parameter: this codebase's own Go side makes the identical tradeoff at
// the same boundary (internal/backup.Scheduler's own
// scheduledBackupContainerName doc comment), duplicating a small, stable
// shape rather than coupling two independently evolving call sites
// together.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  BackupHistoryRecord,
  TriggerBackupRequest,
} from '../types/backupHistory'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const volumeBackupHistoryKeys = {
  all: (appName: string, volumeName: string) =>
    ['apps', appName, 'volumes', volumeName, 'backups'] as const,
  list: (appName: string, volumeName: string) =>
    [...volumeBackupHistoryKeys.all(appName, volumeName), 'list'] as const,
}

const RUNNING_POLL_INTERVAL_MS = 3_000

// Mirrors the server's defaultBackupHistoryLimit, shared with the
// database path (internal/api/backups.go).
export const VOLUME_BACKUP_HISTORY_PAGE_SIZE = 50

function volumeBackupsPath(appName: string, volumeName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/backups`
}

// GET /api/v1/apps/{name}/volumes/{volume}/backups, newest first.
// opts.before cursor-paginates backward, mirroring fetchBackupHistory's
// identical contract.
export async function fetchVolumeBackupHistory(
  appName: string,
  volumeName: string,
  opts: { limit?: number; before?: string } = {},
): Promise<BackupHistoryRecord[]> {
  const params = new URLSearchParams()
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.before) params.set('before', opts.before)
  const qs = params.toString()
  const res = await fetch(
    `${volumeBackupsPath(appName, volumeName)}${qs ? `?${qs}` : ''}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch volume backup history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as BackupHistoryRecord[] | null
  return body ?? []
}

// GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download
// (handleDownloadVolumeBackup). Not a TanStack Query fetcher, the same
// "plain browser navigation target, not fetch/useQuery" reasoning
// backupDownloadURL's own doc comment gives for the raw file stream.
export function volumeBackupDownloadURL(
  appName: string,
  volumeName: string,
  historyId: string,
): string {
  return `${volumeBackupsPath(appName, volumeName)}/${encodeURIComponent(historyId)}/download`
}

export function volumeBackupHistoryListQueryOptions(
  appName: string,
  volumeName: string,
) {
  return queryOptions({
    queryKey: volumeBackupHistoryKeys.list(appName, volumeName),
    queryFn: () => fetchVolumeBackupHistory(appName, volumeName),
  })
}

// Plain useQuery, not suspense/loader-primed, the same reasoning
// useBackupHistory's own doc comment gives.
export function useVolumeBackupHistory(appName: string, volumeName: string) {
  return useQuery({
    ...volumeBackupHistoryListQueryOptions(appName, volumeName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/apps/{name}/volumes/{volume}/backups
// (handleTriggerVolumeBackup). Returns 202 with a placeholder
// backupHistoryResource, the same shape triggerBackup's own doc comment
// describes.
export async function triggerVolumeBackup(
  appName: string,
  volumeName: string,
  req: TriggerBackupRequest,
): Promise<BackupHistoryRecord> {
  const res = await fetch(volumeBackupsPath(appName, volumeName), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Backups require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `trigger volume backup failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as BackupHistoryRecord
}

export function useTriggerVolumeBackup(appName: string, volumeName: string) {
  const queryClient = useQueryClient()
  return useMutation<BackupHistoryRecord, ApiError, string>({
    mutationFn: (targetId: string) =>
      triggerVolumeBackup(appName, volumeName, { target_id: targetId }),
    onSuccess: (record) => {
      queryClient.setQueryData(
        volumeBackupHistoryKeys.list(appName, volumeName),
        (existing: BackupHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: volumeBackupHistoryKeys.list(appName, volumeName),
      })
    },
  })
}
