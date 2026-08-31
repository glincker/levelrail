// Query-key factory and fetchers for the backup verification sub-resource
// of one backup attempt:
// POST/GET /api/v1/databases/{name}/backups/{historyId}/verify(ications)
// (internal/api/backup_verify.go). Kept in its own module, the same
// reasoning queries/restoreHistory.ts's own header comment gives for
// staying separate from queries/backupHistory.ts: a genuinely different
// resource, scoped under a backup attempt rather than a database.

import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { BackupVerificationRecord } from '../types/backupVerification'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const backupVerificationKeys = {
  all: (databaseName: string, backupHistoryId: string) =>
    ['databases', databaseName, 'backups', backupHistoryId, 'verifications'] as const,
  list: (databaseName: string, backupHistoryId: string) =>
    [...backupVerificationKeys.all(databaseName, backupHistoryId), 'list'] as const,
}

// Same polling cadence as useBackupHistory's own RUNNING_POLL_INTERVAL_MS
// (queries/backupHistory.ts): a verification attempt is a download-sized
// amount of I/O against the same kind of bucket a backup itself uploads
// to, so there's no reason to expect a different "how long until this
// finishes" answer.
const RUNNING_POLL_INTERVAL_MS = 3_000

// GET /api/v1/databases/{name}/backups/{historyId}/verifications, newest
// first, per that handler's own doc comment.
export async function fetchBackupVerifications(
  databaseName: string,
  backupHistoryId: string,
): Promise<BackupVerificationRecord[]> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/backups/${encodeURIComponent(backupHistoryId)}/verifications`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch backup verifications failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as BackupVerificationRecord[] | null
  return body ?? []
}

// Plain useQuery, not suspense/loader-primed, the same reasoning
// useBackupHistory's own doc comment gives: an optional, per-row detail,
// not something the route loader needs warm before first paint.
// refetchInterval follows the identical "poll only while the newest
// attempt is still running" shape. enabled lets a caller (the backup
// history table) skip firing this at all for a row it isn't rendering a
// verification badge for.
export function useLatestBackupVerification(
  databaseName: string,
  backupHistoryId: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: backupVerificationKeys.list(databaseName, backupHistoryId),
    queryFn: () => fetchBackupVerifications(databaseName, backupHistoryId),
    enabled,
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/backups/{historyId}/verify
// (handleVerifyBackup). Returns 202 with a placeholder
// backupVerificationResource: status "running", the same placeholder
// shape triggerBackup's own doc comment describes for its endpoint. 501
// carries the same "no master key configured" gap; 404/400/409 mean the
// backup doesn't exist, belongs to a different database, or never
// succeeded, none of which the verify button itself can normally reach
// since it only ever renders for a succeeded row already loaded from this
// same database.
export async function verifyBackup(
  databaseName: string,
  backupHistoryId: string,
): Promise<BackupVerificationRecord> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(databaseName)}/backups/${encodeURIComponent(backupHistoryId)}/verify`,
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
      await readErrorMessage(res, `verify backup failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupVerificationRecord
}

// On success, the placeholder record is written straight into this
// backup's own verification list cache, the same "don't wait on a
// refetch for the row that was just created" reasoning
// useTriggerBackup's own doc comment gives, then the query is invalidated
// so the follow-up refetch (and this hook's own polling once it sees
// "running") picks up from real server state.
export function useVerifyBackup(databaseName: string, backupHistoryId: string) {
  const queryClient = useQueryClient()
  return useMutation<BackupVerificationRecord, ApiError, void>({
    mutationFn: () => verifyBackup(databaseName, backupHistoryId),
    onSuccess: (record) => {
      queryClient.setQueryData(
        backupVerificationKeys.list(databaseName, backupHistoryId),
        (existing: BackupVerificationRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: backupVerificationKeys.list(databaseName, backupHistoryId),
      })
    },
  })
}
