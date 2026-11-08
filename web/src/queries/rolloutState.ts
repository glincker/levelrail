// GET /api/v1/apps/{name}/rollout/state (internal/api/routes_rollout_state.go):
// the newest release's gate, traffic flip, hold countdown and canary.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { Canary } from './canary'

export type RolloutTraffic = 'held' | 'flipped' | 'reverted'

export interface RolloutHold {
  state: string
  mode: string
  started_at?: string
  ends_at?: string
  remaining_seconds: number
  window_seconds: number
  reason?: string
  message?: string
  notice?: string
}

export interface RolloutState {
  app: string
  strategy: string
  deploy_id?: string
  image?: string
  stage: string
  traffic: RolloutTraffic
  gate_failures: number
  auto_rollback: boolean
  flipped_at?: string
  rolled_back_at?: string
  failure_reason?: string
  hold: RolloutHold
  canary?: Canary
}

const POLL_MS = Number(import.meta.env.VITE_ROLLOUT_STATE_POLL_MS) || 5000

export const rolloutStateKeys = {
  one: (appName: string) =>
    [...appKeys.detail(appName), 'rollout-state'] as const,
}

/** A 404 means an older server without the route: show nothing. */
export async function fetchRolloutState(
  appName: string,
): Promise<RolloutState | null> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/rollout/state`,
  )
  if (res.status === 404) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `rollout state failed: ${res.status}`),
    )
  }
  return (await res.json()) as RolloutState
}

export function rolloutStateQueryOptions(appName: string) {
  return queryOptions({
    queryKey: rolloutStateKeys.one(appName),
    queryFn: () => fetchRolloutState(appName),
    // Poll only while a hold or gate is still moving.
    refetchInterval: (q) => {
      const s = q.state.data
      if (!s) return false
      const live = s.hold.state === 'watching' || s.stage === 'gating'
      return live ? POLL_MS : false
    },
  })
}

export function useRolloutState(appName: string) {
  return useQuery(rolloutStateQueryOptions(appName))
}
