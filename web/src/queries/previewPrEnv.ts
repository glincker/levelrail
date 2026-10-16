// GET/POST /api/v1/apps/{name}/previews/{number}/env and
// DELETE .../env/{id} (internal/api/preview_pr_env.go): env overrides for one
// pull request's preview. A secret's value is never returned.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PrPreviewEnvOverride {
  id: string
  key: string
  value: string
  secret: boolean
  updated_at: string
}

export interface SetPrPreviewEnvInput {
  key: string
  value: string
  secret: boolean
}

export const prPreviewEnvKeys = {
  list: (appName: string, prNumber: number) =>
    ['pr-preview-env', appName, prNumber] as const,
}

function basePath(appName: string, prNumber: number): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/previews/${prNumber}/env`
}

export function usePrPreviewEnv(
  appName: string,
  prNumber: number,
  enabled: boolean,
) {
  return useQuery({
    queryKey: prPreviewEnvKeys.list(appName, prNumber),
    enabled,
    queryFn: async (): Promise<PrPreviewEnvOverride[]> => {
      const res = await fetch(basePath(appName, prNumber))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `list pr env failed: ${res.status}`),
        )
      }
      return (await res.json()) as PrPreviewEnvOverride[]
    },
  })
}

export function useSetPrPreviewEnv(appName: string, prNumber: number) {
  const queryClient = useQueryClient()
  return useMutation<PrPreviewEnvOverride, ApiError, SetPrPreviewEnvInput>({
    mutationFn: async (input) => {
      const res = await fetch(basePath(appName, prNumber), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `set pr env failed: ${res.status}`),
        )
      }
      return (await res.json()) as PrPreviewEnvOverride
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: prPreviewEnvKeys.list(appName, prNumber),
      })
    },
  })
}

export function useDeletePrPreviewEnv(appName: string, prNumber: number) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      const res = await fetch(
        `${basePath(appName, prNumber)}/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
      )
      if (res.status === 204) {
        return
      }
      throw new ApiError(
        res.status,
        await readErrorMessage(res, `delete pr env failed: ${res.status}`),
      )
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: prPreviewEnvKeys.list(appName, prNumber),
      })
    },
  })
}
