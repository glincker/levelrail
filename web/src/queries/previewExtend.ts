// POST /api/v1/apps/{name}/previews/{number}/extend
// (internal/api/preview_lifecycle_policy.go): push one preview's expiry out.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { PreviewEnvironment } from '../types/previewEnvironment'
import { previewEnvironmentKeys } from './previewEnvironments'

export interface ExtendPreviewArgs {
  prNumber: number
  hours: number
}

export function useExtendPreviewEnvironment(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<PreviewEnvironment, ApiError, ExtendPreviewArgs>({
    mutationFn: async ({ prNumber, hours }) => {
      const res = await fetch(
        `/api/v1/apps/${encodeURIComponent(appName)}/previews/${prNumber}/extend`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ hours }),
        },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `extend preview failed: ${res.status}`),
        )
      }
      return (await res.json()) as PreviewEnvironment
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: previewEnvironmentKeys.list(appName),
      })
    },
  })
}
