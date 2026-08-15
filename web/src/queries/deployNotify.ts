// Query-key factory and fetchers for GET/POST
// /api/v1/apps/{name}/deploy-notify-targets and DELETE
// .../deploy-notify-targets/{id} (internal/api/deploy_notify_targets.go,
// wave-2 roadmap item #5). Kept in its own module for the same reason
// queries/alerts.ts is: a genuinely different resource shape than
// AppDetail, nested under the same app name, and distinct from
// queries/alerts.ts itself since deploy-outcome notify targets and
// alert rules are two separate backend concepts (see
// internal/alerting/deploy_notify.go's own doc comment) even though
// their CRUD shape looks similar.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  CreateDeployNotifyTargetRequest,
  DeployNotifyTarget,
} from '../types/deployNotify'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const deployNotifyTargetKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'deploy-notify-targets'] as const,
  list: (appName: string) => [...deployNotifyTargetKeys.all(appName), 'list'] as const,
}

// GET /api/v1/apps/{name}/deploy-notify-targets. Returns every target
// scoped to this app, enabled and disabled
// (handleListDeployNotifyTargets's own doc comment), so this fetcher
// does no client-side filtering either.
export async function fetchDeployNotifyTargets(
  appName: string,
): Promise<DeployNotifyTarget[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploy-notify-targets`,
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'deploy notifications are not configured on this control plane',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy notify targets failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployNotifyTarget[]
}

export function deployNotifyTargetListQueryOptions(appName: string) {
  return queryOptions({
    queryKey: deployNotifyTargetKeys.list(appName),
    queryFn: () => fetchDeployNotifyTargets(appName),
  })
}

export function useDeployNotifyTargets(appName: string) {
  return useQuery(deployNotifyTargetListQueryOptions(appName))
}

// POST /api/v1/apps/{name}/deploy-notify-targets
// (handleCreateDeployNotifyTarget). Returns the full server-assigned
// target (id, resource_id), invalidated back into the list query on
// success rather than optimistically appended, the same reasoning
// queries/alerts.ts's own createAlertRule doc comment gives.
export async function createDeployNotifyTarget(
  appName: string,
  req: CreateDeployNotifyTargetRequest,
): Promise<DeployNotifyTarget> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploy-notify-targets`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create deploy notify target failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployNotifyTarget
}

export function useCreateDeployNotifyTarget(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateDeployNotifyTargetRequest) =>
      createDeployNotifyTarget(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: deployNotifyTargetKeys.list(appName),
      })
    },
  })
}

// DELETE /api/v1/apps/{name}/deploy-notify-targets/{id}
// (handleDeleteDeployNotifyTarget). 204 on success, no body to parse.
export async function deleteDeployNotifyTarget(
  appName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploy-notify-targets/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete deploy notify target failed: ${res.status}`),
    )
  }
}

export function useDeleteDeployNotifyTarget(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteDeployNotifyTarget(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: deployNotifyTargetKeys.list(appName),
      })
    },
  })
}
