// Query-key factory and fetcher for the instance-wide backup history
// view: GET /api/v1/backups (internal/api/backups.go's own
// handleListAllBackups), aggregating every database and app volume
// backup into one newest-first list. Mirrors queries/backupHistory.ts's
// shape for the single-database resource, kept as its own module for the
// same reasoning that file's own header comment gives for staying
// separate from queries/apps.ts: a genuinely different query (no
// resource name to scope by) even though the wire shape is identical.

import { queryOptions, useQuery } from '@tanstack/react-query'
import type { BackupHistoryRecord } from '../types/backupHistory'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const allBackupHistoryKeys = {
  all: ['backups'] as const,
  list: () => [...allBackupHistoryKeys.all, 'list'] as const,
}

const RUNNING_POLL_INTERVAL_MS = 3_000

// Mirrors the server's defaultBackupHistoryLimit, shared with the
// per-resource lists (internal/api/backups.go).
export const ALL_BACKUP_HISTORY_PAGE_SIZE = 50

// GET /api/v1/backups, newest first across every resource. opts.before
// cursor-paginates backward, mirroring fetchBackupHistory's identical
// contract.
export async function fetchAllBackupHistory(
  opts: { limit?: number; before?: string } = {},
): Promise<BackupHistoryRecord[]> {
  const params = new URLSearchParams()
  if (opts.limit) params.set('limit', String(opts.limit))
  if (opts.before) params.set('before', opts.before)
  const qs = params.toString()
  const res = await fetch(`/api/v1/backups${qs ? `?${qs}` : ''}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch all backup history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as BackupHistoryRecord[] | null
  return body ?? []
}

export function allBackupHistoryQueryOptions() {
  return queryOptions({
    queryKey: allBackupHistoryKeys.list(),
    queryFn: () => fetchAllBackupHistory(),
  })
}

// Plain useQuery, not suspense/loader-primed, the same reasoning
// useBackupHistory's own doc comment gives; unlike that hook's route
// (routes/backups/index.tsx does prime the first page via its own
// loader, see that file), this one exists for callers that just want the
// live list without owning a route loader.
export function useAllBackupHistory() {
  return useQuery({
    ...allBackupHistoryQueryOptions(),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}
