// Query-key factory and fetchers for the restore history sub-resource of
// an app service's named volume:
// GET/POST /api/v1/apps/{name}/volumes/{volume}/restore(s)
// (internal/api/app_volume_restore.go). Mirrors queries/restoreHistory.ts's
// exact shape, the same "own module, own boundary" reasoning
// queries/volumeBackupHistory.ts's own header comment gives.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  RestoreHistoryRecord,
  TriggerRestoreRequest,
} from '../types/restoreHistory'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { volumeBackupHistoryKeys } from './volumeBackupHistory'

export const volumeRestoreHistoryKeys = {
  all: (appName: string, volumeName: string) =>
    ['apps', appName, 'volumes', volumeName, 'restores'] as const,
  list: (appName: string, volumeName: string) =>
    [...volumeRestoreHistoryKeys.all(appName, volumeName), 'list'] as const,
}

const RUNNING_POLL_INTERVAL_MS = 3_000

function volumeRestoreTriggerPath(appName: string, volumeName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/restore`
}

function volumeRestoreHistoryPath(appName: string, volumeName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/restores`
}

// GET /api/v1/apps/{name}/volumes/{volume}/restores
// (handleListVolumeRestoreHistory), newest first per that handler's own
// doc comment.
export async function fetchVolumeRestoreHistory(
  appName: string,
  volumeName: string,
): Promise<RestoreHistoryRecord[]> {
  const res = await fetch(volumeRestoreHistoryPath(appName, volumeName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch volume restore history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as RestoreHistoryRecord[] | null
  return body ?? []
}

export function volumeRestoreHistoryListQueryOptions(
  appName: string,
  volumeName: string,
) {
  return queryOptions({
    queryKey: volumeRestoreHistoryKeys.list(appName, volumeName),
    queryFn: () => fetchVolumeRestoreHistory(appName, volumeName),
  })
}

export function useVolumeRestoreHistory(appName: string, volumeName: string) {
  return useQuery({
    ...volumeRestoreHistoryListQueryOptions(appName, volumeName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/apps/{name}/volumes/{volume}/restore
// (handleTriggerVolumeRestore). Returns 202 with a placeholder
// restoreHistoryResource, the same shape triggerRestore's own doc
// comment describes. The single most destructive endpoint this app
// calls; callers must gate this behind their own explicit confirmation
// before ever reaching it (see RestoreVolumeBackupDialog).
export async function triggerVolumeRestore(
  appName: string,
  volumeName: string,
  req: TriggerRestoreRequest,
): Promise<RestoreHistoryRecord> {
  const res = await fetch(volumeRestoreTriggerPath(appName, volumeName), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Restores require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `trigger volume restore failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RestoreHistoryRecord
}

// On success, invalidates this volume's own restore list and its backup
// list, the same "a viewer watching a restore in progress must also see
// the backup table stay in sync" reasoning useTriggerRestore's own doc
// comment gives (nothing about backup_history itself actually changes,
// but a stale-looking neighbor table would read as a bug).
export function useTriggerVolumeRestore(appName: string, volumeName: string) {
  const queryClient = useQueryClient()
  return useMutation<RestoreHistoryRecord, ApiError, string>({
    mutationFn: (backupId: string) =>
      triggerVolumeRestore(appName, volumeName, { backup_id: backupId }),
    onSuccess: (record) => {
      queryClient.setQueryData(
        volumeRestoreHistoryKeys.list(appName, volumeName),
        (existing: RestoreHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: volumeRestoreHistoryKeys.list(appName, volumeName),
      })
      void queryClient.invalidateQueries({
        queryKey: volumeBackupHistoryKeys.list(appName, volumeName),
      })
    },
  })
}
