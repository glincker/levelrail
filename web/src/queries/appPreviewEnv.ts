// Mutation hooks for one app's preview-specific env var overrides: PUT
// /api/v1/apps/{name}/preview-env/{key} (declare or replace), DELETE
// .../preview-env/{key} (remove). internal/api/apps_preview_env.go is
// the backend side. No GET here: the declarations already live on
// AppDetail.preview_env_overrides from GET /api/v1/apps/{name}, the same
// "no separate list endpoint" shape queries/appVaultEnv.ts's own doc
// comment already establishes for vault_env.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { AppDetail } from '../types/appDetail'

export interface SetAppPreviewEnvOverrideInput {
  key: string
  value: string
}

// handleSetAppPreviewEnvOverride returns { key, value } on success, 404
// if the app doesn't exist.
export async function setAppPreviewEnvOverride(
  appName: string,
  input: SetAppPreviewEnvOverrideInput,
): Promise<{ key: string; value: string }> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/preview-env/${encodeURIComponent(input.key)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: input.value }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set preview env override failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as { key: string; value: string }
}

// Updates the cached AppDetail's preview_env_overrides map directly
// rather than invalidating and refetching: the response already carries
// the exact value that was just saved, the same reasoning
// useSetAppVaultEnv already uses.
export function useSetAppPreviewEnvOverride(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetAppPreviewEnvOverrideInput) =>
      setAppPreviewEnvOverride(appName, input),
    onSuccess: (saved) => {
      queryClient.setQueryData<AppDetail>(appKeys.detail(appName), (app) =>
        app
          ? {
              ...app,
              preview_env_overrides: {
                ...app.preview_env_overrides,
                [saved.key]: saved.value,
              },
            }
          : app,
      )
    },
  })
}

// handleClearAppPreviewEnvOverride returns 204 on success (idempotent:
// clearing an undeclared key is not an error), 404 if the app doesn't
// exist.
export async function clearAppPreviewEnvOverride(
  appName: string,
  key: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/preview-env/${encodeURIComponent(key)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `clear preview env override failed: ${res.status}`,
    ),
  )
}

export function useClearAppPreviewEnvOverride(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (key: string) => clearAppPreviewEnvOverride(appName, key),
    onSuccess: (_void, key) => {
      queryClient.setQueryData<AppDetail>(appKeys.detail(appName), (app) => {
        if (!app?.preview_env_overrides) return app
        const rest = { ...app.preview_env_overrides }
        delete rest[key]
        return { ...app, preview_env_overrides: rest }
      })
    },
  })
}
