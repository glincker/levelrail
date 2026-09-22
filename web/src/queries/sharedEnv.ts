// Query/mutation hooks for the combined and secret-only shared env var
// endpoints (internal/api/shared_env_secrets.go): GET .../env/all (every
// shared var at a project/organization/environment scope, plain and
// secret-marked alike, a secret entry's value always ""), PUT/DELETE
// .../env/secrets/{key} (set/remove one secret-marked var). The plain
// full-replace GET/PUT .../env endpoints have their own existing query
// modules (queries/projectEnv.ts, organizationEnv.ts, environmentEnv.ts)
// this file does not duplicate.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { SecretsNotConfiguredError } from './secrets'

export type SharedEnvScope = 'project' | 'organization' | 'environment'

export interface SharedEnvVar {
  key: string
  value: string
  secret: boolean
  // updatedAt/stale are only populated when secret is true: a plain
  // shared var's own updated_at reflects the last full-replace PUT, not
  // a rotation-relevant event, matching the backend's own
  // sharedEnvVarResource shape (internal/api/shared_env_secrets.go).
  updatedAt?: string
  stale?: boolean
}

interface SharedEnvVarResource {
  key: string
  value: string
  secret: boolean
  updated_at?: string
  stale?: boolean
}

function scopePath(scope: SharedEnvScope, id: string): string {
  const collection =
    scope === 'project'
      ? 'projects'
      : scope === 'organization'
        ? 'organizations'
        : 'environments'
  return `/api/v1/${collection}/${encodeURIComponent(id)}`
}

export const sharedEnvKeys = {
  all: ['shared-env'] as const,
  list: (scope: SharedEnvScope, id: string) =>
    [...sharedEnvKeys.all, 'list', scope, id] as const,
}

// Fetches every shared var at scope/id, plain and secret-marked alike.
// GET /api/v1/{scope}s/{id}/env/all.
export async function fetchSharedEnvAll(
  scope: SharedEnvScope,
  id: string,
): Promise<SharedEnvVar[]> {
  const res = await fetch(`${scopePath(scope, id)}/env/all`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list shared env vars failed: ${res.status}`),
    )
  }
  const resources = (await res.json()) as SharedEnvVarResource[]
  return resources.map((r) => ({
    key: r.key,
    value: r.value,
    secret: r.secret,
    updatedAt: r.updated_at,
    stale: r.stale,
  }))
}

export function useSharedEnvAll(scope: SharedEnvScope, id: string) {
  return useQuery({
    queryKey: sharedEnvKeys.list(scope, id),
    queryFn: () => fetchSharedEnvAll(scope, id),
    enabled: Boolean(id),
  })
}

export interface SetSharedEnvSecretInput {
  key: string
  value: string
}

// PUT /api/v1/{scope}s/{id}/env/secrets/{key}: encrypts value and marks
// key secret. 204 with no body on success, matching setSecret's own
// "never echo a value back" shape for per-app secrets.
export async function setSharedEnvSecret(
  scope: SharedEnvScope,
  id: string,
  input: SetSharedEnvSecretInput,
): Promise<void> {
  const res = await fetch(
    `${scopePath(scope, id)}/env/secrets/${encodeURIComponent(input.key)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: input.value }),
    },
  )
  if (res.status === 204) {
    return
  }
  if (res.status === 501) {
    throw new SecretsNotConfiguredError()
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `set shared env secret failed: ${res.status}`),
  )
}

export function useSetSharedEnvSecret(scope: SharedEnvScope, id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetSharedEnvSecretInput) =>
      setSharedEnvSecret(scope, id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: sharedEnvKeys.list(scope, id),
      })
    },
  })
}

// DELETE /api/v1/{scope}s/{id}/env/secrets/{key}.
export async function deleteSharedEnvSecret(
  scope: SharedEnvScope,
  id: string,
  key: string,
): Promise<void> {
  const res = await fetch(
    `${scopePath(scope, id)}/env/secrets/${encodeURIComponent(key)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `delete shared env secret failed: ${res.status}`,
    ),
  )
}

export function useDeleteSharedEnvSecret(scope: SharedEnvScope, id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => deleteSharedEnvSecret(scope, id, key),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: sharedEnvKeys.list(scope, id),
      })
    },
  })
}
