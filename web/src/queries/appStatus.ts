import { queryOptions, useQuery } from '@tanstack/react-query'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

// GET /api/v1/apps/{name}/status (internal/api/deploys.go handleAppStatus).
export interface AppStatus {
  conditions: ReconcileCondition[]
  deploy_id?: string
  outcome?: DeployAttempt['outcome']
  outcome_reason?: string
  rollout_crash_loop_restarts?: number
}

export const appStatusKeys = {
  status: (appName: string) => [...appKeys.detail(appName), 'status'] as const,
}

/** A 404 means an older server without the route: no outcome, local rules apply. */
export async function fetchAppStatus(appName: string): Promise<AppStatus> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/status`)
  if (res.status === 404) return { conditions: [] }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app status failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as AppStatus | null
  return { ...body, conditions: body?.conditions ?? [] }
}

export function appStatusQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appStatusKeys.status(appName),
    queryFn: () => fetchAppStatus(appName),
    refetchInterval: 10_000,
  })
}

export function useAppStatus(appName: string) {
  return useQuery(appStatusQueryOptions(appName))
}
