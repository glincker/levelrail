// Query/mutation hooks for one app's branch-scoped env var overrides:
// GET /api/v1/apps/{name}/branch-env (list), POST .../branch-env
// (declare or replace, by branch_pattern+key), DELETE
// .../branch-env/{id} (remove by the override's own id).
// internal/api/apps_branch_env.go is the backend side. A narrower,
// pattern-matched sibling of queries/appPreviewEnv.ts's own unscoped
// override: this one only applies when a preview's own branch matches.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appBranchEnvKeys = {
  all: ['app-branch-env'] as const,
  list: (appName: string) =>
    [...appBranchEnvKeys.all, 'list', appName] as const,
}

export interface AppBranchEnvOverride {
  id: string
  branchPattern: string
  key: string
  // value is always "" for a secret-marked entry, never sent by the
  // server for one, matching SecretsEditor's own "never echo a value
  // back" rule.
  value: string
  secret: boolean
  updatedAt: string
}

interface AppBranchEnvOverrideResource {
  id: string
  branch_pattern: string
  key: string
  value: string
  secret: boolean
  updated_at: string
}

function fromResource(r: AppBranchEnvOverrideResource): AppBranchEnvOverride {
  return {
    id: r.id,
    branchPattern: r.branch_pattern,
    key: r.key,
    value: r.value,
    secret: r.secret,
    updatedAt: r.updated_at,
  }
}

export async function fetchAppBranchEnvOverrides(
  appName: string,
): Promise<AppBranchEnvOverride[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/branch-env`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `list branch env overrides failed: ${res.status}`,
      ),
    )
  }
  const resources = (await res.json()) as AppBranchEnvOverrideResource[]
  return resources.map(fromResource)
}

export function useAppBranchEnvOverrides(appName: string) {
  return useQuery({
    queryKey: appBranchEnvKeys.list(appName),
    queryFn: () => fetchAppBranchEnvOverrides(appName),
  })
}

export interface SetAppBranchEnvOverrideInput {
  branchPattern: string
  key: string
  value: string
  secret: boolean
}

// handleSetAppBranchEnv returns the saved override (200), 400 for a
// missing branch_pattern/key or a malformed pattern, 404 if the app
// doesn't exist, 501 if secret is true and no master key is configured
// on this control plane.
export async function setAppBranchEnvOverride(
  appName: string,
  input: SetAppBranchEnvOverrideInput,
): Promise<AppBranchEnvOverride> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/branch-env`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        branch_pattern: input.branchPattern,
        key: input.key,
        value: input.value,
        secret: input.secret,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set branch env override failed: ${res.status}`,
      ),
    )
  }
  return fromResource((await res.json()) as AppBranchEnvOverrideResource)
}

export function useSetAppBranchEnvOverride(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: SetAppBranchEnvOverrideInput) =>
      setAppBranchEnvOverride(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appBranchEnvKeys.list(appName),
      })
    },
  })
}

// handleDeleteAppBranchEnv returns 204 on success, 404 if id doesn't
// exist under this app.
export async function deleteAppBranchEnvOverride(
  appName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/branch-env/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `delete branch env override failed: ${res.status}`,
    ),
  )
}

export function useDeleteAppBranchEnvOverride(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteAppBranchEnvOverride(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appBranchEnvKeys.list(appName),
      })
    },
  })
}
