// Query-key factory and fetchers for the restore history sub-resource of
// a database: GET/POST /api/v1/databases/{name}/restore(s)
// (internal/api/restore.go). Kept in its own module rather than folded
// into queries/backupHistory.ts, the same reasoning that module's own
// header comment gives for staying separate from queries/backupTargets.ts:
// a genuinely different resource shape and a different endpoint pair,
// even though the two are related and this module leans on
// backupHistoryKeys.list to invalidate the backup list once a restore
// finishes (a restore doesn't change backup_history rows, but the
// database's live data changing is exactly the kind of state a viewer
// looking at this page cares about being reflected promptly elsewhere).

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

export const restoreHistoryKeys = {
  all: (databaseName: string) =>
    ['databases', databaseName, 'restores'] as const,
  list: (databaseName: string) =>
    [...restoreHistoryKeys.all(databaseName), 'list'] as const,
}

// Same polling cadence as useBackupHistory's own RUNNING_POLL_INTERVAL_MS
// (queries/backupHistory.ts): a restore and a backup are both a
// dump-sized amount of I/O against the same kind of small managed
// database, so there's no reason to expect the "how long until this
// finishes" answer to differ between the two directions.
const RUNNING_POLL_INTERVAL_MS = 3_000

// GET /api/v1/databases/{name}/restores (handleListRestoreHistory).
// Ordered newest first by the server already, per that handler's own
// doc comment, matching fetchBackupHistory's identical claim about its
// own endpoint.
export async function fetchRestoreHistory(
  databaseName: string,
): Promise<RestoreHistoryRecord[]> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/restores`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch restore history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as RestoreHistoryRecord[] | null
  return body ?? []
}

export function restoreHistoryListQueryOptions(databaseName: string) {
  return queryOptions({
    queryKey: restoreHistoryKeys.list(databaseName),
    queryFn: () => fetchRestoreHistory(databaseName),
  })
}

// Plain useQuery, not suspense/loader-primed, the same reasoning
// useBackupHistory's own doc comment gives: an optional section on the
// database detail page, not something the route loader needs warm before
// first paint. refetchInterval follows the identical "poll only while
// the newest row is still running" shape.
export function useRestoreHistory(databaseName: string) {
  return useQuery({
    ...restoreHistoryListQueryOptions(databaseName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/restore (handleTriggerRestore). Returns
// 202 with a placeholder restoreHistoryResource: status "running", no
// finished_at yet, the same placeholder shape triggerBackup's own doc
// comment describes for its endpoint. 501 carries the same "no master
// key configured" gap; 404 means the database or the named backup does
// not exist; 400 means backup_id was omitted or names a backup taken
// from a different database; 409 means the named backup exists but its
// own Status was never "succeeded", so there is nothing safe to restore
// from.
export async function triggerRestore(
  databaseName: string,
  req: TriggerRestoreRequest,
): Promise<RestoreHistoryRecord> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/restore`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Restores require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `trigger restore failed: ${res.status}`),
    )
  }
  return (await res.json()) as RestoreHistoryRecord
}

// On success, the placeholder record is written straight into the
// restore history list's cache, the same "don't wait on a refetch for
// the row that was just created" reasoning useTriggerBackup's own doc
// comment gives, then both the restore list and the backup list are
// invalidated: the restore list for the obvious reason, the backup list
// because a database whose data a viewer is actively restoring is a
// database whose page they're actively watching, and a stale backup
// history table sitting next to a freshly-updating restore table would
// read as a bug even though nothing about backup_history itself changed.
export function useTriggerRestore(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<RestoreHistoryRecord, ApiError, string>({
    mutationFn: (backupId: string) =>
      triggerRestore(databaseName, { backup_id: backupId }),
    onSuccess: (record) => {
      queryClient.setQueryData(
        restoreHistoryKeys.list(databaseName),
        (existing: RestoreHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: restoreHistoryKeys.list(databaseName),
      })
    },
  })
}
