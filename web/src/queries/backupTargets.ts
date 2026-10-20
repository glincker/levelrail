// Query-key factory and fetchers for the /backup-targets resource
// (internal/api/backup_targets.go), following the same shared-
// queryOptions pattern queries/tokens.ts and queries/alerts.ts already
// established: one options object per query, used by both a route
// loader's queryClient.ensureQueryData call and the component's
// useSuspenseQuery/hook call.
//
// This only covers connecting/listing/removing a bucket destination
// (backup_targets.go's own handlers stop at the connection, not the
// runs). Triggering an actual backup and reading its history is
// queries/backupHistory.ts's job, a separate resource scoped under a
// database name rather than a target id.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  BackupTarget,
  CreateBackupTargetRequest,
} from '../types/backupTarget'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const backupTargetKeys = {
  all: ['backup-targets'] as const,
  list: () => [...backupTargetKeys.all, 'list'] as const,
}

// GET /api/v1/backup-targets (handleListBackupTargets). Unlike create,
// this handler never checks for a configured master key, so it has no
// 501 case: it just lists whatever rows already exist.
export async function fetchBackupTargets(): Promise<BackupTarget[]> {
  const res = await fetch('/api/v1/backup-targets')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch backup targets failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupTarget[]
}

export function backupTargetListQueryOptions() {
  return queryOptions({
    queryKey: backupTargetKeys.list(),
    queryFn: fetchBackupTargets,
  })
}

export function useBackupTargets() {
  return useSuspenseQuery(backupTargetListQueryOptions())
}

// Non-suspending counterpart to useBackupTargets, mirroring
// queries/projects.ts's useProjectListOptional exactly and for the
// identical reason: StorageAttachmentCard renders on the app Overview
// route, which has no Suspense boundary of its own around this list, so
// a failure to list backup targets must degrade to "no targets
// available" (an empty array while loading or on error) rather than
// crash the whole Overview page.
export function useBackupTargetsOptional() {
  return useQuery({ ...backupTargetListQueryOptions(), retry: false })
}

// POST /api/v1/backup-targets (handleCreateBackupTarget). 501 means the
// control plane was started without APP_MASTER_KEY: rt.backupSecrets is
// nil and the handler bails before touching the request body at all, the
// same "server configuration gap, not a bad submission" case
// queries/secrets.ts's SecretsNotConfiguredError and queries/alerts.ts's
// 501 branch both carry. Surfaced as a plain ApiError with status 501
// here (matching alerts.ts/metrics.ts's newer convention) rather than a
// bespoke exception subclass: callers narrow on `error.status === 501`.
export async function createBackupTarget(
  req: CreateBackupTargetRequest,
): Promise<BackupTarget> {
  const res = await fetch('/api/v1/backup-targets', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Backup targets require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create backup target failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupTarget
}

export function useCreateBackupTarget() {
  const queryClient = useQueryClient()
  return useMutation<BackupTarget, ApiError, CreateBackupTargetRequest>({
    mutationFn: createBackupTarget,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupTargetKeys.list() })
    },
  })
}

// DELETE /api/v1/backup-targets/{id} (handleDeleteBackupTarget). 204 on
// success, no body to parse. 404 if the id is already gone, 409 if
// backup_history still references it (nothing writes history yet, so
// this can't actually happen today, but the handler's own doc comment
// is explicit that it's a real, reachable response code once it does,
// not a hypothetical, so it gets the same "let the real status/message
// through" treatment as every other failure here rather than being
// special-cased).
export async function deleteBackupTarget(id: string): Promise<void> {
  const res = await fetch(`/api/v1/backup-targets/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `delete backup target failed: ${res.status}`),
  )
}

export function useDeleteBackupTarget() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteBackupTarget,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupTargetKeys.list() })
    },
  })
}

// POST /api/v1/backup-targets/{id}/test (handleTestBackupTarget): probes
// the target's stored credentials against its configured bucket, without
// uploading or deleting anything. 204 on success, 502 with a specific
// reason on failure, the same shape registryCredentials.ts's
// testRegistryCredential already establishes for its own on-demand
// verification.
export async function testBackupTarget(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/backup-targets/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `test backup target failed: ${res.status}`),
  )
}

export function useTestBackupTarget() {
  return useMutation<void, ApiError, string>({
    mutationFn: testBackupTarget,
  })
}
