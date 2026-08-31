// Query-key factory and fetchers for the "restore as new database"
// sub-resource of a database: GET/POST /api/v1/databases/{name}/
// clone-restores and /restore-as-new (internal/api/database_clone_
// restore.go). Mirrors queries/restoreHistory.ts's own shape exactly,
// the non-destructive counterpart to that in-place restore action.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  CloneRestoreRecord,
  TriggerCloneRestoreRequest,
} from '../types/cloneRestore'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

export const cloneRestoreKeys = {
  all: (sourceDatabaseName: string) =>
    ['databases', sourceDatabaseName, 'clone-restores'] as const,
  list: (sourceDatabaseName: string) =>
    [...cloneRestoreKeys.all(sourceDatabaseName), 'list'] as const,
}

// Same polling cadence as useRestoreHistory's own RUNNING_POLL_INTERVAL_MS
// (queries/restoreHistory.ts): a clone-restore is a dump-sized amount of
// I/O plus a wait for the new database to come up, no reason to expect a
// different "how long until this finishes" answer.
const RUNNING_POLL_INTERVAL_MS = 3_000

// GET /api/v1/databases/{name}/clone-restores (handleListCloneRestores).
// Ordered newest first by the server already, per that handler's own
// doc comment.
export async function fetchCloneRestores(
  sourceDatabaseName: string,
): Promise<CloneRestoreRecord[]> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(sourceDatabaseName)}/clone-restores`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch clone restore history failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as CloneRestoreRecord[] | null
  return body ?? []
}

export function cloneRestoreListQueryOptions(sourceDatabaseName: string) {
  return queryOptions({
    queryKey: cloneRestoreKeys.list(sourceDatabaseName),
    queryFn: () => fetchCloneRestores(sourceDatabaseName),
  })
}

// Plain useQuery, the same "optional section, not something the loader
// needs warm before first paint" shape useRestoreHistory already
// establishes.
export function useCloneRestores(sourceDatabaseName: string) {
  return useQuery({
    ...cloneRestoreListQueryOptions(sourceDatabaseName),
    refetchInterval: (query) => {
      const latest = query.state.data?.[0]
      return latest?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false
    },
  })
}

// POST /api/v1/databases/{name}/restore-as-new (handleCloneRestore).
// Returns 202 with a placeholder cloneRestoreResource: status "running",
// no finished_at yet, the same placeholder shape triggerRestore's own
// doc comment describes for its endpoint. 501 carries the same "no
// master key configured" gap; 404 means the source database or the
// named backup does not exist; 400 means backup_id/new_name was omitted,
// new_name collides with the source's own name, or the backup came from
// a different database; 409 means new_name already exists or the named
// backup never succeeded.
export async function triggerCloneRestore(
  sourceDatabaseName: string,
  req: TriggerCloneRestoreRequest,
): Promise<CloneRestoreRecord> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(sourceDatabaseName)}/restore-as-new`,
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
      await readErrorMessage(res, `restore as new database failed: ${res.status}`),
    )
  }
  return (await res.json()) as CloneRestoreRecord
}

// On success, the placeholder record is written straight into the
// clone-restore list's cache, the same reasoning useTriggerRestore's own
// doc comment gives, then the database list is invalidated too: a brand
// new database now exists and the list page shouldn't need a manual
// refresh to show it.
export function useTriggerCloneRestore(sourceDatabaseName: string) {
  const queryClient = useQueryClient()
  return useMutation<CloneRestoreRecord, ApiError, TriggerCloneRestoreRequest>({
    mutationFn: (req: TriggerCloneRestoreRequest) =>
      triggerCloneRestore(sourceDatabaseName, req),
    onSuccess: (record) => {
      queryClient.setQueryData(
        cloneRestoreKeys.list(sourceDatabaseName),
        (existing: CloneRestoreRecord[] | undefined) => [
          record,
          ...(existing ?? []),
        ],
      )
      void queryClient.invalidateQueries({
        queryKey: cloneRestoreKeys.list(sourceDatabaseName),
      })
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}
