// Query-key factory and fetchers for the backup history sub-resource of
// a database: GET/POST /api/v1/databases/{name}/backups
// (internal/api/backups.go). Kept in its own module rather than folded
// into queries/backupTargets.ts, the same reasoning queries/deploys.ts
// gives for staying separate from queries/apps.ts: a genuinely different
// resource shape (one backup attempt, not a bucket connection) scoped
// under a different parent (a database name, not the target itself),
// even though the two are related. queries/backupTargets.ts's own header
// comment used to note that triggering a backup and reading its history
// had no corresponding endpoint yet; this module is what closes that gap.

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

export const backupHistoryKeys = {
  all: (databaseName: string) =>
    ['databases', databaseName, 'backups'] as const,
  list: (databaseName: string) =>
    [...backupHistoryKeys.all(databaseName), 'list'] as const,
}

// Only polled while the most recently triggered attempt is still
// "running": a few seconds, matching how quickly a dump-and-upload of a
// small database actually completes, without hammering the control
// plane for the rest of this section's lifetime once nothing is in
// flight.
const RUNNING_POLL_INTERVAL_MS = 3_000

// Mirrors the server's defaultBackupHistoryLimit: a page shorter than
// this means there's nothing older left to load.
export const BACKUP_HISTORY_PAGE_SIZE = 50

// GET /api/v1/databases/{name}/backups, newest first. opts.before
// cursor-paginates backward, mirroring fetchAuditLog's ?before contract.
export async function fetchBackupHistory(
  databaseName: string,
  opts: { limit?: number; before?: string } = {},
): Promise<BackupHistoryRecord[]> {
  const params = new URLSearchParams()
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.before) params.set('before', opts.before)
  const qs = params.toString()
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/backups${qs ? `?${qs}` : ''}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch backup history failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as BackupHistoryRecord[] | null
  return body ?? []
}

// GET /api/v1/databases/{name}/backups/{historyId}/download
// (handleDownloadBackup). Not a TanStack Query fetcher: the response is a
// raw file stream, not JSON, so this is consumed as a plain browser
// navigation target (an <a href> with a download attribute) rather than
// through fetch/useQuery. Auth rides along the same way every other
// same-origin request in this app already does, the httpOnly session
// cookie (see web/src/lib/authStore.ts), so no token needs to be embedded
// in the URL.
export function backupDownloadURL(
  databaseName: string,
  historyId: string,
): string {
  return `/api/v1/databases/${encodeURIComponent(databaseName)}/backups/${encodeURIComponent(historyId)}/download`
}

export function backupHistoryListQueryOptions(databaseName: string) {
  return queryOptions({
    queryKey: backupHistoryKeys.list(databaseName),
    queryFn: () => fetchBackupHistory(databaseName),
  })
}

// Plain useQuery, not suspense/loader-primed: the same reasoning
// queries/alerts.ts's useAlertRules gives, this is an optional section on
// the database detail page, not something the route loader needs warm
// before first paint.
//
// refetchInterval is a function of the latest cached data rather than a
// fixed interval like queries/activity.ts's ACTIVITY_POLL_INTERVAL_MS: a
// trigger response is only ever a placeholder (see fetchBackupHistory's
// own comment above and triggerBackup's below), so the real outcome is
// discovered by polling this list until the newest row is no longer
// "running", then going idle again rather than polling forever.
export function useBackupHistory(databaseName: string) {
  return useQuery({
    ...backupHistoryListQueryOptions(databaseName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/backups (handleTriggerBackup). Returns
// 202 with a placeholder backupHistoryResource: status "running", no
// object_key/size_bytes/finished_at yet (types/backupHistory.ts's own
// comment), since the actual dump and upload happen asynchronously after
// this call returns. 501 carries the same "no master key configured on
// this control plane" gap createBackupTarget's own 501 branch already
// handles (backup credentials cannot be decrypted without one either);
// 400 means target_id was omitted; 404 means the database or the target
// itself does not exist.
export async function triggerBackup(
  databaseName: string,
  req: TriggerBackupRequest,
): Promise<BackupHistoryRecord> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/backups`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Backups require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `trigger backup failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupHistoryRecord
}

// On success, the placeholder record is written straight into the
// history list's cache (same "don't wait on a refetch for the row that
// was just created" reasoning useCreateDatabase gives) so the running
// attempt shows up immediately, then the query is invalidated so the
// follow-up refetch (and useBackupHistory's own polling once it sees
// "running") picks up from the real server state rather than trusting
// this optimistic write indefinitely.
export function useTriggerBackup(databaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<BackupHistoryRecord, ApiError, string>({
    mutationFn: (targetId: string) =>
      triggerBackup(databaseName, { target_id: targetId }),
    onSuccess: (record) => {
      queryClient.setQueryData(
        backupHistoryKeys.list(databaseName),
        (existing: BackupHistoryRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: backupHistoryKeys.list(databaseName),
      })
    },
  })
}
