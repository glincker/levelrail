// Query-key factory, fetchers, and mutation hook for
// GET /api/v1/apps/{name}/promote/preview and POST
// /api/v1/apps/{name}/promote (internal/api/promote.go). Kept in its own
// module, the same "genuinely different resource" reasoning
// queries/deployCompare.ts's own doc comment already gives for deploys.ts.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { PromotePreviewResource } from '../types/promote'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const promoteKeys = {
  all: ['promote'] as const,
  preview: (appName: string, to: string, target: string) =>
    [...promoteKeys.all, 'preview', appName, to, target] as const,
}

export async function fetchPromotePreview(
  appName: string,
  to: string,
  target: string,
): Promise<PromotePreviewResource> {
  const params = new URLSearchParams({ to })
  if (target) {
    params.set('target', target)
  }
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/promote/preview?${params.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `preview promotion failed: ${res.status}`),
    )
  }
  return (await res.json()) as PromotePreviewResource
}

// Not a suspense query: this feeds PromoteAppDialog's own preview panel,
// which only exists once an environment is picked inside an already-open
// dialog, the same "enabled once a prerequisite is chosen" shape
// useEnvironmentListOptional already has for its own dialog.
export function usePromotePreview(
  appName: string,
  to: string,
  target: string,
) {
  return useQuery({
    queryKey: promoteKeys.preview(appName, to, target),
    queryFn: () => fetchPromotePreview(appName, to, target),
    enabled: to !== '',
    retry: false,
  })
}

interface PromoteAppInput {
  to: string
  target: string
  // confirm must be true to promote into a protected environment
  // (internal/api's environmentNeedsConfirmation); derived from
  // PromoteAppDialog's own ProtectedEnvironmentNotice acknowledgment.
  confirm?: boolean
}

export async function promoteApp(
  appName: string,
  input: PromoteAppInput,
): Promise<{ name: string; image: string }> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/promote`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        to: input.to,
        target: input.target || undefined,
        confirm: input.confirm ?? false,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `promote app failed: ${res.status}`),
    )
  }
  return (await res.json()) as { name: string; image: string }
}

// The mutated app is the *target*, not appName itself (promote.go moves
// appName's image onto a sibling app), so invalidation targets whatever
// name the response actually reports, the same "trust the response, not
// the caller's own name" shape useSetAppEnvironment already has for its
// own updated.name.
export function usePromoteApp(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: PromoteAppInput) => promoteApp(appName, input),
    onSuccess: (updated) => {
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(updated.name),
      })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(updated.name),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(updated.name),
      })
    },
  })
}
