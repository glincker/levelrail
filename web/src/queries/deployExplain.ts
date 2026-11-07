// GET /api/v1/deployments/{id}/explain (internal/api/routes_obs_explain.go):
// a deterministic account of why a deploy failed, built from its timeline.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DeployExplainEvent {
  id: string
  at: string
  phase: string
  status: string
  code: string
  title: string
  detail?: string
  reason?: string
}

export interface DeployExplanation {
  deploy_id: string
  app: string
  verdict: string
  failed: boolean
  phase?: string
  code?: string
  headline: string
  cause?: string
  hint?: string
  evidence?: DeployExplainEvent[]
  last_logs?: string[]
  rolled_back?: boolean
}

export const deployExplainKeys = {
  one: (deployId: string) => ['deploy-explain', deployId] as const,
}

export function deployExplainQueryOptions(deployId: string, enabled: boolean) {
  return queryOptions({
    queryKey: deployExplainKeys.one(deployId),
    queryFn: async (): Promise<DeployExplanation | null> => {
      const res = await fetch(
        `/api/v1/deployments/${encodeURIComponent(deployId)}/explain`,
      )
      if (res.status === 404) return null
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `explain deploy failed: ${res.status}`),
        )
      }
      return (await res.json()) as DeployExplanation
    },
    enabled,
  })
}

/** Lazy: only a failed deploy asks for its explanation. */
export function useDeployExplanation(deployId: string, enabled: boolean) {
  return useQuery(deployExplainQueryOptions(deployId, enabled))
}
