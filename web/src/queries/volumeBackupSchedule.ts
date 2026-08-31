// Query-key factory and fetchers for an app service volume's scheduled
// backup config:
// GET/PUT/DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule
// (internal/api/app_volume_backups.go). Unlike a database (whose
// schedule fields ride along on GET .../databases/{name} itself,
// queries/databases.ts's useSetDatabaseBackupSchedule/
// useClearDatabaseBackupSchedule), a service can have any number of
// volumes, so there is no single app resource to embed one volume's
// schedule into: this is a real, standalone GET/PUT/DELETE sub-resource
// with its own query key, not a patch onto the app detail cache.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface VolumeBackupScheduleRecord {
  service_name: string
  volume_name: string
  target_id?: string
  schedule?: string
  retain?: number
  retain_days?: number
}

export interface SetVolumeBackupScheduleRequest {
  target_id: string
  schedule: string
  retain?: number
  retain_days?: number
}

export const volumeBackupScheduleKeys = {
  detail: (appName: string, volumeName: string) =>
    ['apps', appName, 'volumes', volumeName, 'backup-schedule'] as const,
}

function volumeBackupSchedulePath(appName: string, volumeName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/volumes/${encodeURIComponent(volumeName)}/backup-schedule`
}

export async function fetchVolumeBackupSchedule(
  appName: string,
  volumeName: string,
): Promise<VolumeBackupScheduleRecord> {
  const res = await fetch(volumeBackupSchedulePath(appName, volumeName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch volume backup schedule failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as VolumeBackupScheduleRecord
}

export function volumeBackupScheduleQueryOptions(
  appName: string,
  volumeName: string,
) {
  return queryOptions({
    queryKey: volumeBackupScheduleKeys.detail(appName, volumeName),
    queryFn: () => fetchVolumeBackupSchedule(appName, volumeName),
  })
}

// Plain useQuery, not suspense/loader-primed: an optional section on the
// app detail page, the same reasoning every other backup-related hook in
// this codebase already follows.
export function useVolumeBackupSchedule(appName: string, volumeName: string) {
  return useQuery(volumeBackupScheduleQueryOptions(appName, volumeName))
}

export async function setVolumeBackupSchedule(
  appName: string,
  volumeName: string,
  req: SetVolumeBackupScheduleRequest,
): Promise<VolumeBackupScheduleRecord> {
  const res = await fetch(volumeBackupSchedulePath(appName, volumeName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set volume backup schedule failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as VolumeBackupScheduleRecord
}

export function useSetVolumeBackupSchedule(
  appName: string,
  volumeName: string,
) {
  const queryClient = useQueryClient()
  return useMutation<
    VolumeBackupScheduleRecord,
    ApiError,
    SetVolumeBackupScheduleRequest
  >({
    mutationFn: (req) => setVolumeBackupSchedule(appName, volumeName, req),
    onSuccess: (record) => {
      queryClient.setQueryData(
        volumeBackupScheduleKeys.detail(appName, volumeName),
        record,
      )
    },
  })
}

export async function clearVolumeBackupSchedule(
  appName: string,
  volumeName: string,
): Promise<void> {
  const res = await fetch(volumeBackupSchedulePath(appName, volumeName), {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear volume backup schedule failed: ${res.status}`,
      ),
    )
  }
}

export function useClearVolumeBackupSchedule(
  appName: string,
  volumeName: string,
) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: () => clearVolumeBackupSchedule(appName, volumeName),
    onSuccess: () => {
      queryClient.setQueryData(
        volumeBackupScheduleKeys.detail(appName, volumeName),
        (existing: VolumeBackupScheduleRecord | undefined) =>
          existing && {
            service_name: existing.service_name,
            volume_name: existing.volume_name,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: volumeBackupScheduleKeys.detail(appName, volumeName),
      })
    },
  })
}
