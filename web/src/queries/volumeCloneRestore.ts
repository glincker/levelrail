// Query-key factory and fetchers for the "restore as new volume"
// sub-resource of an app service volume: GET/POST /api/v1/apps/{name}/
// volumes/{volume}/clone-restores and /restore-as-new
// (internal/api/app_volume_clone_restore.go). Mirrors queries/
// cloneRestore.ts's own shape exactly, the non-destructive counterpart to
// queries/volumeRestoreHistory.ts's in-place restore.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  TriggerVolumeCloneRestoreRequest,
  VolumeCloneRestoreRecord,
} from '../types/volumeCloneRestore'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { volumeBackupHistoryKeys } from './volumeBackupHistory'

export const volumeCloneRestoreKeys = {
  all: (appName: string, volumeName: string) =>
    ['apps', appName, 'volumes', volumeName, 'clone-restores'] as const,
  list: (appName: string, volumeName: string) =>
    [...volumeCloneRestoreKeys.all(appName, volumeName), 'list'] as const,
}

// Same polling cadence as useCloneRestores' own RUNNING_POLL_INTERVAL_MS
// (queries/cloneRestore.ts).
const RUNNING_POLL_INTERVAL_MS = 3_000

function volumeCloneRestorePath(appName: string, volumeName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/restore-as-new`
}

function volumeCloneRestoreListPath(
  appName: string,
  volumeName: string,
): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/clone-restores`
}

// GET /api/v1/apps/{name}/volumes/{volume}/clone-restores
// (handleListVolumeCloneRestores). Ordered newest first by the server
// already, per that handler's own doc comment.
export async function fetchVolumeCloneRestores(
  appName: string,
  volumeName: string,
): Promise<VolumeCloneRestoreRecord[]> {
  const res = await fetch(volumeCloneRestoreListPath(appName, volumeName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch volume clone restore history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as VolumeCloneRestoreRecord[] | null
  return body ?? []
}

export function volumeCloneRestoreListQueryOptions(
  appName: string,
  volumeName: string,
) {
  return queryOptions({
    queryKey: volumeCloneRestoreKeys.list(appName, volumeName),
    queryFn: () => fetchVolumeCloneRestores(appName, volumeName),
  })
}

export function useVolumeCloneRestores(appName: string, volumeName: string) {
  return useQuery({
    ...volumeCloneRestoreListQueryOptions(appName, volumeName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new
// (handleVolumeCloneRestore). Returns 202 with a placeholder
// volumeCloneRestoreResource, the same placeholder shape
// triggerCloneRestore's own doc comment describes.
export async function triggerVolumeCloneRestore(
  appName: string,
  volumeName: string,
  req: TriggerVolumeCloneRestoreRequest,
): Promise<VolumeCloneRestoreRecord> {
  const res = await fetch(volumeCloneRestorePath(appName, volumeName), {
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
        `restore as new volume failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as VolumeCloneRestoreRecord
}

// On success, the placeholder record is written straight into the
// clone-restore list's cache, the same reasoning useTriggerCloneRestore's
// own doc comment gives, then this volume's own backup list is
// invalidated too, the same "keep a neighbor table from reading stale"
// reasoning useTriggerVolumeRestore's own doc comment gives.
export function useTriggerVolumeCloneRestore(
  appName: string,
  volumeName: string,
) {
  const queryClient = useQueryClient()
  return useMutation<
    VolumeCloneRestoreRecord,
    ApiError,
    TriggerVolumeCloneRestoreRequest
  >({
    mutationFn: (req: TriggerVolumeCloneRestoreRequest) =>
      triggerVolumeCloneRestore(appName, volumeName, req),
    onSuccess: (record) => {
      queryClient.setQueryData(
        volumeCloneRestoreKeys.list(appName, volumeName),
        (existing: VolumeCloneRestoreRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: volumeCloneRestoreKeys.list(appName, volumeName),
      })
      void queryClient.invalidateQueries({
        queryKey: volumeBackupHistoryKeys.list(appName, volumeName),
      })
    },
  })
}
