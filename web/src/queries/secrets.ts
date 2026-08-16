// Query/mutation hooks for the secrets sub-resource of an app: GET
// /api/v1/apps/{name}/secrets (key names + locked state, never values),
// PUT /api/v1/apps/{name}/secrets/{key} (set/rotate a value, reversible
// per-key lock guard), POST .../secrets/{key}/lock (toggle a key's
// lock). internal/api/secrets.go's handlers are the backend side of all
// three.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Thrown when the control plane responds 501, meaning it was started
// without APP_MASTER_KEY set: the secrets handlers check rt.secrets ==
// nil before doing anything else. Distinct from every other failure
// these calls can produce because it is a server configuration gap, not
// a bad key/value, missing app, or lock conflict, so callers should
// render it as an explanation rather than a form error.
export class SecretsNotConfiguredError extends Error {
  constructor() {
    super(
      'secrets are not configured on this control plane (no master key set)',
    )
    this.name = 'SecretsNotConfiguredError'
  }
}

export const secretKeys = {
  all: ['secrets'] as const,
  list: (appName: string) => [...secretKeys.all, 'list', appName] as const,
}

export interface SecretKeyState {
  key: string
  locked: boolean
}

// Fetches every known secret key for an app, with its locked state,
// never a value. GET /api/v1/apps/{name}/secrets.
export async function fetchSecretKeys(
  appName: string,
): Promise<SecretKeyState[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/secrets`,
  )
  if (res.status === 501) {
    throw new SecretsNotConfiguredError()
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list secret keys failed: ${res.status}`),
    )
  }
  return (await res.json()) as SecretKeyState[]
}

export function useSecretKeys(appName: string) {
  return useQuery({
    queryKey: secretKeys.list(appName),
    queryFn: () => fetchSecretKeys(appName),
    retry: (failureCount, error) =>
      !(error instanceof SecretsNotConfiguredError) && failureCount < 2,
  })
}

export interface SetSecretInput {
  key: string
  value: string
  overwriteLocked?: boolean
}

// handleSetSecret returns 204 with no body on success (the value just
// submitted is already known to the caller, echoing it back would be an
// unnecessary leak surface), 501 if no master key is configured, 404 if
// the app does not exist, 409 if the key is locked and overwriteLocked
// wasn't set, and 400 for an empty value or malformed body.
export async function setSecret(
  appName: string,
  input: SetSecretInput,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/secrets/${encodeURIComponent(input.key)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        value: input.value,
        overwrite_locked: input.overwriteLocked ?? false,
      }),
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
    await readErrorMessage(res, `set secret failed: ${res.status}`),
  )
}

// Invalidates the key list on success (a new key, or an existing key's
// presence, is now known) even though the mutation itself never returns
// the list -- same "invalidate after write" shape every other mutation
// in queries/apps.ts uses.
export function useSetSecret(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetSecretInput) => setSecret(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: secretKeys.list(appName) })
    },
  })
}

export interface SetSecretLockInput {
  key: string
  locked: boolean
}

// POST /api/v1/apps/{name}/secrets/{key}/lock. 404 if no value has been
// set for that key yet (you can't lock a key that doesn't exist).
export async function setSecretLock(
  appName: string,
  input: SetSecretLockInput,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/secrets/${encodeURIComponent(input.key)}/lock`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ locked: input.locked }),
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
    await readErrorMessage(res, `set secret lock failed: ${res.status}`),
  )
}

export function useSetSecretLock(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetSecretLockInput) => setSecretLock(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: secretKeys.list(appName) })
    },
  })
}
