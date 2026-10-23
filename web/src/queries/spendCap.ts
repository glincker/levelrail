// /api/v1/apps/{name}/budget (internal/api/spend_caps.go): a monthly egress
// and cost cap with alerts and an optional throttle or pause.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

import type { SpendGuardMode } from '../types/costExplorer'

export type SpendCapAction = 'none' | 'throttle' | 'pause'
export type { SpendGuardMode }

export interface SpendCapCondition {
  healthy: boolean
  reason: string
  message: string
}

export interface SpendForecast {
  cost: { mid: number; confidence: string }
  egress_bytes: number
  pct: number
  will_exceed: boolean
}

export interface SpendGuardOverride {
  reason: string
  actor: string
  expires_at: string
}

export interface SpendGuard {
  blocked: boolean
  allowed: boolean
  reasons: { source: string; pct: number }[]
  override?: SpendGuardOverride
}

export interface SpendCap {
  app: string
  guard_mode: SpendGuardMode
  forecast?: SpendForecast
  guard?: SpendGuard
  monthly_egress_bytes: number
  monthly_cost_ceiling: number
  thresholds: number[]
  action: SpendCapAction
  throttle_rps: number
  period: string
  egress_bytes: number
  cost: number
  egress_pct: number
  cost_pct: number
  pct: number
  enforced: SpendCapAction
  resets_at: string
  condition: SpendCapCondition
  cost_priced: boolean
}

export interface SpendCapUpdate {
  monthly_egress_bytes: number
  monthly_cost_ceiling: number
  thresholds: number[]
  action: SpendCapAction
  throttle_rps: number
  guard_mode?: SpendGuardMode
}

export interface PausePreview {
  apps: {
    name: string
    replicas: number
    daily_run_rate: number
    estimated_saving: number
  }[]
  total_replicas: number
  remaining_days: number
  estimated_saving: number
  currency: string
  note: string
}

export interface SpendCapEvent {
  id: number
  period: string
  kind: string
  detail: string
  pct: number
  at: string
}

export const spendCapKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'budget'] as const,
  history: (appName: string) =>
    [...appKeys.detail(appName), 'budget-history'] as const,
}

function path(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/budget${suffix}`
}

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

/** A null result means the app has no budget yet, which is not an error. */
async function getSpendCap(appName: string): Promise<SpendCap | null> {
  const res = await fetch(path(appName))
  if (res.status === 404) return null
  if (!res.ok) return fail(res, 'budget request')
  return (await res.json()) as SpendCap
}

async function putSpendCap(
  appName: string,
  body: SpendCapUpdate,
): Promise<SpendCap> {
  const res = await fetch(path(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) return fail(res, 'budget save')
  return (await res.json()) as SpendCap
}

async function deleteSpendCap(appName: string): Promise<void> {
  const res = await fetch(path(appName), { method: 'DELETE' })
  if (!res.ok) return fail(res, 'budget removal')
}

async function getHistory(appName: string): Promise<SpendCapEvent[]> {
  const res = await fetch(path(appName, '/history?limit=20'))
  if (!res.ok) return fail(res, 'budget history')
  const body = (await res.json()) as { events: SpendCapEvent[] }
  return body.events
}

export function spendCapQueryOptions(appName: string) {
  return queryOptions({
    queryKey: spendCapKeys.detail(appName),
    queryFn: () => getSpendCap(appName),
    refetchInterval: 60_000,
    retry: false,
  })
}

export function spendCapHistoryQueryOptions(appName: string) {
  return queryOptions({
    queryKey: spendCapKeys.history(appName),
    queryFn: () => getHistory(appName),
    retry: false,
  })
}

export function useSpendCapActions(appName: string) {
  const queryClient = useQueryClient()
  const refreshHistory = () =>
    queryClient.invalidateQueries({ queryKey: spendCapKeys.history(appName) })
  const save = useMutation<SpendCap, ApiError, SpendCapUpdate>({
    mutationFn: (body) => putSpendCap(appName, body),
    onSuccess: (cap) => {
      queryClient.setQueryData(spendCapKeys.detail(appName), cap)
      void refreshHistory()
    },
  })
  const clear = useMutation<void, ApiError, void>({
    mutationFn: () => deleteSpendCap(appName),
    onSuccess: () => {
      queryClient.setQueryData(spendCapKeys.detail(appName), null)
      void refreshHistory()
    },
  })
  return { save, clear }
}

export function usePausePreview(url: string) {
  return useMutation<PausePreview, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(url, { method: 'POST' })
      if (!res.ok) return fail(res, 'pause preview')
      return (await res.json()) as PausePreview
    },
  })
}

export function useOverrideSpendGuard(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<SpendGuard, ApiError, string>({
    mutationFn: async (reason) => {
      const res = await fetch(path(appName, '/override'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason }),
      })
      if (!res.ok) return fail(res, 'deploy override')
      return (await res.json()) as SpendGuard
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: spendCapKeys.detail(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: spendCapKeys.history(appName),
      })
    },
  })
}

export function appPauseUrl(appName: string): string {
  return path(appName, '/dry-run')
}

export function budgetPauseUrl(budgetId: string): string {
  return `/api/v1/costs/budgets/${encodeURIComponent(budgetId)}/dry-run`
}
