// Query and mutation for GET /api/v1/system/secrets/binding and POST
// /api/v1/system/secrets/rebind (internal/api/secret_binding.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface SecretBindingStatus {
  total: number
  bound: number
  legacy: number
}

export interface SecretRebindFailure {
  owner: string
  key: string
  reason: string
}

export interface SecretRebindResult {
  scanned: number
  rebound: number
  alreadyBound: number
  changed: number
  failedCount: number
  failed: SecretRebindFailure[]
  remaining: number
}

export const secretBindingKeys = {
  status: ['secret-binding'] as const,
}

// null means the control plane has no master key (501).
async function fetchSecretBinding(): Promise<SecretBindingStatus | null> {
  const res = await fetch('/api/v1/system/secrets/binding')
  if (res.status === 501) {
    return null
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `secret binding status failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as SecretBindingStatus
}

export function useSecretBinding() {
  return useQuery({
    queryKey: secretBindingKeys.status,
    queryFn: fetchSecretBinding,
  })
}

async function rebindSecrets(): Promise<SecretRebindResult> {
  const res = await fetch('/api/v1/system/secrets/rebind', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `rebind secrets failed: ${res.status}`),
    )
  }
  return (await res.json()) as SecretRebindResult
}

export function useRebindSecrets() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: rebindSecrets,
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: secretBindingKeys.status }),
  })
}
