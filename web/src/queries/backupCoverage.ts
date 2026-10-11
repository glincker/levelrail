// Coverage, protect-all and orphaned data (internal/api/backups_coverage.go,
// internal/api/storage_orphans.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  BackupCoverage,
  CoverageEncryption,
  ProtectAllRequest,
  ProtectAllResponse,
  StorageOrphans,
} from '../types/backupCoverage'

export const backupCoverageKeys = {
  all: ['backup-coverage'] as const,
  coverage: () => [...backupCoverageKeys.all, 'coverage'] as const,
  orphans: () => [...backupCoverageKeys.all, 'orphans'] as const,
}

async function failure(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function fetchBackupCoverage(): Promise<BackupCoverage> {
  const res = await fetch('/api/v1/backups/coverage')
  if (!res.ok) return failure(res, 'fetch backup coverage')
  const body = (await res.json()) as BackupCoverage
  return {
    ...body,
    resources: body.resources ?? [],
    targets: body.targets ?? [],
  }
}

export function backupCoverageQueryOptions() {
  return queryOptions({
    queryKey: backupCoverageKeys.coverage(),
    queryFn: fetchBackupCoverage,
  })
}

export function useBackupCoverage() {
  return useQuery(backupCoverageQueryOptions())
}

export async function postProtectAll(
  req: ProtectAllRequest,
): Promise<ProtectAllResponse> {
  const res = await fetch('/api/v1/backups/protect-all', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) return failure(res, 'protect all')
  const body = (await res.json()) as ProtectAllResponse
  return { ...body, plan: body.plan ?? [] }
}

/** useProtectAll plans or applies depending on the request's apply flag. */
export function useProtectAll() {
  const queryClient = useQueryClient()
  return useMutation<ProtectAllResponse, ApiError, ProtectAllRequest>({
    mutationFn: postProtectAll,
    onSuccess: (data) => {
      if (data.applied) {
        void queryClient.invalidateQueries({
          queryKey: backupCoverageKeys.all,
        })
      }
    },
  })
}

export async function fetchStorageOrphans(): Promise<StorageOrphans> {
  const res = await fetch('/api/v1/storage/orphans')
  if (!res.ok) return failure(res, 'fetch orphaned data')
  const body = (await res.json()) as StorageOrphans
  return { ...body, orphans: body.orphans ?? [] }
}

export function useStorageOrphans() {
  return useQuery<StorageOrphans, ApiError>({
    queryKey: backupCoverageKeys.orphans(),
    queryFn: fetchStorageOrphans,
  })
}

export function useReclaimOrphan() {
  const queryClient = useQueryClient()
  return useMutation<
    { removed: string; reclaimed_bytes: number },
    ApiError,
    string
  >({
    mutationFn: async (id) => {
      const res = await fetch(
        `/api/v1/storage/orphans/${encodeURIComponent(id)}`,
        {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ confirm: id }),
        },
      )
      if (!res.ok) return failure(res, 'reclaim orphaned data')
      return (await res.json()) as { removed: string; reclaimed_bytes: number }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: backupCoverageKeys.orphans(),
      })
    },
  })
}

export function useAckBackupKey() {
  const queryClient = useQueryClient()
  return useMutation<CoverageEncryption, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch('/api/v1/backups/encryption-key/ack', {
        method: 'POST',
      })
      if (!res.ok) return failure(res, 'acknowledge backup key')
      return (await res.json()) as CoverageEncryption
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: backupCoverageKeys.coverage(),
      })
    },
  })
}
