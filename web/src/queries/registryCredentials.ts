// Query-key factory and fetchers for the /registry-credentials resource
// (internal/api/registry_credentials.go), following the same shared-
// queryOptions pattern queries/backupTargets.ts already established.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateRegistryCredentialRequest,
  RegistryCredential,
} from '../types/registryCredential'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const registryCredentialKeys = {
  all: ['registry-credentials'] as const,
  list: () => [...registryCredentialKeys.all, 'list'] as const,
}

// GET /api/v1/registry-credentials (handleListRegistryCredentials): no
// 501 case, just lists whatever rows already exist.
export async function fetchRegistryCredentials(): Promise<
  RegistryCredential[]
> {
  const res = await fetch('/api/v1/registry-credentials')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch registry credentials failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RegistryCredential[]
}

export function registryCredentialListQueryOptions() {
  return queryOptions({
    queryKey: registryCredentialKeys.list(),
    queryFn: fetchRegistryCredentials,
  })
}

export function useRegistryCredentials() {
  return useSuspenseQuery(registryCredentialListQueryOptions())
}

// POST /api/v1/registry-credentials (handleCreateRegistryCredential).
// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case backupTargets.ts's createBackupTarget
// carries for the identical reason (both route through internal/secrets).
export async function createRegistryCredential(
  req: CreateRegistryCredentialRequest,
): Promise<RegistryCredential> {
  const res = await fetch('/api/v1/registry-credentials', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Registry credentials require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create registry credential failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RegistryCredential
}

export function useCreateRegistryCredential() {
  const queryClient = useQueryClient()
  return useMutation<
    RegistryCredential,
    ApiError,
    CreateRegistryCredentialRequest
  >({
    mutationFn: createRegistryCredential,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}

// DELETE /api/v1/registry-credentials/{id} (handleDeleteRegistryCredential).
// 204 on success, no body to parse.
export async function deleteRegistryCredential(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `delete registry credential failed: ${res.status}`,
    ),
  )
}

export function useDeleteRegistryCredential() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteRegistryCredential,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}

// POST /api/v1/registry-credentials/{id}/test
// (handleTestRegistryCredential): authenticates the stored credential
// against its registry host, without pulling anything. 204 on success,
// 502 with a specific reason on failure, the same shape
// notificationChannels.ts's testExistingNotificationChannel already
// establishes for its own on-demand verification.
export async function testRegistryCredential(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `test registry credential failed: ${res.status}`,
    ),
  )
}

export function useTestRegistryCredential() {
  return useMutation<void, ApiError, string>({
    mutationFn: testRegistryCredential,
  })
}
