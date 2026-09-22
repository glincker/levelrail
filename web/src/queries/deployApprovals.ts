// Query-key factory, fetchers, and mutation hooks for
// GET/POST /api/v1/deploy-approvals* (internal/api/deploy_approvals.go):
// the two-person approval queue a deploy/promote into a protected
// environment now goes through instead of applying on confirm: true.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  DeployApprovalDecisionResult,
  DeployApprovalListResult,
  DeployApprovalResource,
} from '../types/deployApproval'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { deployAttemptKeys } from './deployAttempts'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployApprovalKeys = {
  all: ['deploy-approvals'] as const,
  list: (status: string, service?: string) =>
    [...deployApprovalKeys.all, 'list', status, service ?? ''] as const,
  detail: (id: string) => [...deployApprovalKeys.all, 'detail', id] as const,
}

export async function fetchDeployApprovals(
  status = 'pending',
  service?: string,
): Promise<DeployApprovalResource[]> {
  const params = new URLSearchParams({ status })
  if (service) {
    params.set('service', service)
  }
  const res = await fetch(`/api/v1/deploy-approvals?${params.toString()}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `list deploy approvals failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as DeployApprovalListResult
  return body.approvals ?? []
}

export function deployApprovalListQueryOptions(
  status = 'pending',
  service?: string,
) {
  return queryOptions({
    queryKey: deployApprovalKeys.list(status, service),
    queryFn: () => fetchDeployApprovals(status, service),
  })
}

// Suspense variant for the dedicated /approvals queue page, which
// commits to loading its own primary data before rendering, the same
// convention useAuditLog's own page establishes.
export function useDeployApprovals(status = 'pending', service?: string) {
  return useSuspenseQuery(deployApprovalListQueryOptions(status, service))
}

// Non-suspense variant for a secondary surface that must never block on
// this query: the app detail page's own pending-approval banner
// (PendingDeployApprovalBanner.tsx) and the sidebar's pending-count
// badge both poll this in the background and simply render nothing
// while it's loading, the same "optional signal, absence is not an
// error" shape useImageTagsOptional already establishes elsewhere.
export function useDeployApprovalsOptional(
  status = 'pending',
  service?: string,
) {
  return useQuery({
    ...deployApprovalListQueryOptions(status, service),
    // A pending queue that matters enough to page an approver also
    // matters enough to notice promptly without a manual refresh; 30s
    // matches this dashboard's other "quietly stay fresh" polls (e.g.
    // ConvergenceIndicator's own reconcile-status poll).
    refetchInterval: 30_000,
  })
}

async function decideDeployApproval(
  id: string,
  verb: 'approve' | 'reject',
  reason?: string,
): Promise<DeployApprovalResource | DeployApprovalDecisionResult> {
  const res = await fetch(
    `/api/v1/deploy-approvals/${encodeURIComponent(id)}/${verb}`,
    {
      method: 'POST',
      headers: reason ? { 'Content-Type': 'application/json' } : undefined,
      body: reason ? JSON.stringify({ reason }) : undefined,
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `${verb} deploy approval failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as
    DeployApprovalResource | DeployApprovalDecisionResult
}

// useApproveDeployApproval invalidates every query an approved deploy's
// underlying action (executeConfirmedDeploy/promote, deploy_approvals.go)
// would otherwise have invalidated directly, since it runs through the
// exact same reconcile path once approved: the app detail cache, its
// deploy status, its deploy-attempt history, and the approvals queue
// itself.
export function useApproveDeployApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      decideDeployApproval(
        id,
        'approve',
      ) as Promise<DeployApprovalDecisionResult>,
    onSuccess: (result) => {
      const name = result.approval.service_name
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: deployKeys.status(name) })
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(name),
      })
      void queryClient.invalidateQueries({ queryKey: deployApprovalKeys.all })
    },
  })
}

export function useRejectDeployApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      decideDeployApproval(
        id,
        'reject',
        reason,
      ) as Promise<DeployApprovalResource>,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: deployApprovalKeys.all })
    },
  })
}
