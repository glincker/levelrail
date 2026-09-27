// Fetchers and hooks for a preview environment's policy and the fork
// approval action (internal/api/preview_environments_policy_handlers.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  PreviewPolicy,
  PreviewPolicyUpdate,
} from '../types/previewEnvironment'
import { previewEnvironmentKeys } from './previewEnvironments'

export const previewPolicyKeys = {
  detail: (appName: string) =>
    [...previewEnvironmentKeys.all, 'policy', appName] as const,
}

function policyUrl(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/preview-policy`
}

export async function fetchPreviewPolicy(
  appName: string,
): Promise<PreviewPolicy> {
  const res = await fetch(policyUrl(appName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch preview policy failed: ${res.status}`),
    )
  }
  return (await res.json()) as PreviewPolicy
}

export function previewPolicyQueryOptions(appName: string) {
  return queryOptions({
    queryKey: previewPolicyKeys.detail(appName),
    queryFn: () => fetchPreviewPolicy(appName),
  })
}

export function usePreviewPolicy(appName: string) {
  return useQuery(previewPolicyQueryOptions(appName))
}

export function useSetPreviewPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<PreviewPolicy, ApiError, PreviewPolicyUpdate>({
    mutationFn: async (update) => {
      const res = await fetch(policyUrl(appName), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(update),
      })
      if (res.status === 404) {
        throw new ApiError(
          404,
          'Connect a git source before changing preview policy.',
        )
      }
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `set preview policy failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as PreviewPolicy
    },
    onSuccess: (policy) => {
      queryClient.setQueryData(previewPolicyKeys.detail(appName), policy)
      void queryClient.invalidateQueries({
        queryKey: previewEnvironmentKeys.list(appName),
      })
    },
  })
}

export function useApprovePreviewEnvironment(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, number>({
    mutationFn: async (prNumber) => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/previews/${prNumber}/approve`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ confirm: true }),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `approve preview failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: previewEnvironmentKeys.list(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: previewPolicyKeys.detail(appName),
      })
    },
  })
}
