// Mutation hooks for one app's Vault-sourced env var declarations: PUT
// /api/v1/apps/{name}/vault-env/{key} (declare or replace), DELETE
// .../vault-env/{key} (remove). internal/api/apps_vault_env.go is the
// backend side. No GET here: the declarations already live on
// AppDetail.vault_env from GET /api/v1/apps/{name}, the same "no
// separate list endpoint" shape appKeys.detail's own cache already
// covers for every other app field.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AppDetail } from '../types/appDetail'

export interface SetAppVaultEnvInput {
  key: string
  path: string
  vaultKey: string
}

// handleSetAppVaultEnv returns the stored { path, key } reference on
// success, 404 if the app doesn't exist, 400 for a malformed body or a
// key already declared in secret_env.
export async function setAppVaultEnv(
  appName: string,
  input: SetAppVaultEnvInput,
): Promise<{ path: string; key: string }> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/vault-env/${encodeURIComponent(input.key)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: input.path, key: input.vaultKey }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set vault env var failed: ${res.status}`),
    )
  }
  return (await res.json()) as { path: string; key: string }
}

// Updates the cached AppDetail's vault_env map directly rather than
// invalidating and refetching: the response already carries the exact
// value that was just saved, the same "we already know the result"
// reasoning queries/apps.ts's useUpdateApp uses for its own
// setQueryData call.
export function useSetAppVaultEnv(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetAppVaultEnvInput) => setAppVaultEnv(appName, input),
    onSuccess: (saved, input) => {
      queryClient.setQueryData<AppDetail>(appKeys.detail(appName), (app) =>
        app
          ? {
              ...app,
              vault_env: { ...app.vault_env, [input.key]: saved },
            }
          : app,
      )
    },
  })
}

// handleClearAppVaultEnv returns 204 on success (idempotent: clearing an
// undeclared key is not an error), 404 if the app doesn't exist.
export async function clearAppVaultEnv(
  appName: string,
  key: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/vault-env/${encodeURIComponent(key)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `clear vault env var failed: ${res.status}`),
  )
}

export function useClearAppVaultEnv(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => clearAppVaultEnv(appName, key),
    onSuccess: (_void, key) => {
      queryClient.setQueryData<AppDetail>(appKeys.detail(appName), (app) => {
        if (!app?.vault_env) return app
        const rest = { ...app.vault_env }
        delete rest[key]
        return { ...app, vault_env: rest }
      })
    },
  })
}
