// Query-key factory, fetchers, and mutation hook for
// GET /api/v1/apps/{name}/promote/preview and POST
// /api/v1/apps/{name}/promote (internal/api/promote.go). Kept in its own
// module, the same "genuinely different resource" reasoning
// queries/deployCompare.ts's own doc comment already gives for deploys.ts.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { PromotePreviewResource } from '../types/promote'
import type { DeployApprovalResource } from '../types/deployApproval'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { deployApprovalKeys } from './deployApprovals'
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
export function usePromotePreview(appName: string, to: string, target: string) {
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

// PromoteAppResult mirrors triggerDeploy's own TriggerDeployResult
// (queries/deploys.ts): a flat { name, image } on the wire for the
// common case, or pending_approval instead when the destination
// environment is protected and this was accepted as a pending approval
// rather than applied.
export type PromoteAppResult =
  { name: string; image: string } | { pending_approval: DeployApprovalResource }

export function isPendingPromoteApproval(
  result: PromoteAppResult,
): result is { pending_approval: DeployApprovalResource } {
  return (
    typeof result === 'object' &&
    result !== null &&
    'pending_approval' in result &&
    result.pending_approval != null
  )
}

export async function promoteApp(
  appName: string,
  input: PromoteAppInput,
): Promise<PromoteAppResult> {
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
  return (await res.json()) as PromoteAppResult
}

// The mutated app is the *target*, not appName itself (promote.go moves
// appName's image onto a sibling app), so invalidation targets whatever
// name the response actually reports, the same "trust the response, not
// the caller's own name" shape useSetAppEnvironment already has for its
// own updated.name. When the result is a pending approval instead
// (isPendingPromoteApproval), nothing promoted yet, so only the
// approvals queue is invalidated.
export function usePromoteApp(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: PromoteAppInput) => promoteApp(appName, input),
    onSuccess: (result) => {
      if (isPendingPromoteApproval(result)) {
        void queryClient.invalidateQueries({
          queryKey: deployApprovalKeys.all,
        })
        return
      }
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(result.name),
      })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(result.name),
      })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(result.name),
      })
    },
  })
}
