// Query-key factories and fetchers for point-in-time restore
// (internal/api/pitr.go): PITR enable/disable/status, base backup
// history and manual trigger, and the PITR restore endpoint and its own
// history. Kept in one module, unlike backup/restore history's own
// separate files, since all three sub-resources here are read together
// on the same section of the database detail page and share the
// identical enabled-gate reasoning.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  BaseBackupHistoryRecord,
  PITRRestoreHistoryRecord,
  PITRStatus,
  TriggerPITRRestoreRequest,
} from '../types/pitr'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const pitrKeys = {
  status: (databaseName: string) =>
    ['databases', databaseName, 'pitr', 'status'] as const,
  baseBackups: (databaseName: string) =>
    ['databases', databaseName, 'pitr', 'base-backups'] as const,
  restores: (databaseName: string) =>
    ['databases', databaseName, 'pitr', 'restores'] as const,
}

// Same polling cadence used everywhere else a running attempt needs to
// self-update (useBackupHistory, useRestoreHistory): 3s is frequent
// enough to feel live without hammering the API.
const RUNNING_POLL_INTERVAL_MS = 3_000

async function fetchJSON<T>(url: string, errorContext: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${errorContext} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function pitrURL(databaseName: string, suffix = ''): string {
  return `/api/v1/databases/${encodeURIComponent(databaseName)}${suffix}`
}

// GET /api/v1/databases/{name}/pitr (handleGetPITRStatus).
export function usePITRStatus(databaseName: string) {
  return useQuery({
    queryKey: pitrKeys.status(databaseName),
    queryFn: () =>
      fetchJSON<PITRStatus>(pitrURL(databaseName, '/pitr'), 'get pitr status'),
    // Refetch while enabled: the recoverable window's own end keeps
    // moving forward as WAL archives, so a picker built from a stale
    // window would silently drift out of date the longer a viewer sits
    // on this page.
    refetchInterval: (query) => (query.state.data?.enabled ? 10_000 : false),
  })
}

// POST /api/v1/databases/{name}/pitr (handleEnablePITR). 400 means the
// database's engine isn't postgres, the only engine PITR supports today.
export function useEnablePITR(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(pitrURL(databaseName, '/pitr'), {
        method: 'POST',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `enable pitr failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: pitrKeys.status(databaseName),
      })
    },
  })
}

// DELETE /api/v1/databases/{name}/pitr (handleDisablePITR).
export function useDisablePITR(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(pitrURL(databaseName, '/pitr'), {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `disable pitr failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: pitrKeys.status(databaseName),
      })
    },
  })
}

// GET /api/v1/databases/{name}/base-backups (handleListBaseBackupHistory).
export function baseBackupHistoryQueryOptions(databaseName: string) {
  return queryOptions({
    queryKey: pitrKeys.baseBackups(databaseName),
    queryFn: () =>
      fetchJSON<BaseBackupHistoryRecord[]>(
        pitrURL(databaseName, '/base-backups'),
        'list base backup history',
      ),
  })
}

export function useBaseBackupHistory(databaseName: string) {
  return useQuery({
    ...baseBackupHistoryQueryOptions(databaseName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/base-backups (handleTriggerBaseBackup).
// 409 means PITR isn't enabled for this database yet.
export async function triggerBaseBackup(
  databaseName: string,
  targetId: string,
): Promise<BaseBackupHistoryRecord> {
  const res = await fetch(pitrURL(databaseName, '/base-backups'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ target_id: targetId }),
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
      await readErrorMessage(res, `trigger base backup failed: ${res.status}`),
    )
  }
  return (await res.json()) as BaseBackupHistoryRecord
}

export function useTriggerBaseBackup(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<BaseBackupHistoryRecord, ApiError, string>({
    mutationFn: (targetId: string) => triggerBaseBackup(databaseName, targetId),
    onSuccess: (record) => {
      queryClient.setQueryData(
        pitrKeys.baseBackups(databaseName),
        (existing: BaseBackupHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: pitrKeys.baseBackups(databaseName),
      })
    },
  })
}

// GET /api/v1/databases/{name}/pitr-restores (handleListPITRRestoreHistory).
export function usePITRRestoreHistory(databaseName: string) {
  return useQuery({
    queryKey: pitrKeys.restores(databaseName),
    queryFn: () =>
      fetchJSON<PITRRestoreHistoryRecord[]>(
        pitrURL(databaseName, '/pitr-restores'),
        'list pitr restore history',
      ),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/pitr-restore (handleTriggerPITRRestore).
// 409 covers both "PITR not enabled" and "target_time outside the
// currently recoverable window"; the response body's message
// distinguishes the two for display.
export async function triggerPITRRestore(
  databaseName: string,
  req: TriggerPITRRestoreRequest,
): Promise<PITRRestoreHistoryRecord> {
  const res = await fetch(pitrURL(databaseName, '/pitr-restore'), {
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
      await readErrorMessage(res, `trigger pitr restore failed: ${res.status}`),
    )
  }
  return (await res.json()) as PITRRestoreHistoryRecord
}

export function useTriggerPITRRestore(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    PITRRestoreHistoryRecord,
    ApiError,
    TriggerPITRRestoreRequest
  >({
    mutationFn: (req) => triggerPITRRestore(databaseName, req),
    onSuccess: (record) => {
      queryClient.setQueryData(
        pitrKeys.restores(databaseName),
        (existing: PITRRestoreHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: pitrKeys.restores(databaseName),
      })
    },
  })
}
