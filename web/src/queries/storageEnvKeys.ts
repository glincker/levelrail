// Query-key factory and fetcher for GET /api/v1/storage-env-keys
// (internal/api/storage_env_keys.go's handleListStorageEnvKeys): every
// env var name attaching storage to an app can inject, backed by
// internal/reconcile/application.StorageEnvKeys rather than a hardcoded
// frontend list. StorageAttachmentCard reads this instead of
// hand-maintaining its own copy, the same pattern queries/databaseEngines.ts
// already establishes for the database engine registry, so the two lists
// can never drift apart.

import { useQuery, queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const storageEnvKeysKeys = {
  all: ['storage-env-keys'] as const,
}

export async function fetchStorageEnvKeys(): Promise<string[]> {
  const res = await fetch('/api/v1/storage-env-keys')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch storage env keys failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as string[]
}

// staleTime matches databaseEnginesQueryOptions's own reasoning: this
// list changes only when the control plane itself is upgraded, not
// during a normal session.
export function storageEnvKeysQueryOptions() {
  return queryOptions({
    queryKey: storageEnvKeysKeys.all,
    queryFn: fetchStorageEnvKeys,
    staleTime: 60_000,
  })
}

// Not suspense: backs a pre-attach warning, not a route's primary
// content, the same "must never block this card's core function"
// reasoning useDatabaseEnginesOptional's own doc comment establishes.
// A failure or slow load here just means the collision warning
// momentarily doesn't show, never a blocked attach flow.
export function useStorageEnvKeysOptional() {
  return useQuery({ ...storageEnvKeysQueryOptions(), retry: false })
}
