// /api/v1/apps/{name}/autoscale (internal/api/autoscale.go): a replica
// autoscaler policy with the live replica count and a history of decisions.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type AutoscaleMetric =
  'cpu_percent' | 'memory_percent' | 'requests_per_replica'

export interface AutoscaleCondition {
  healthy: boolean
  reason: string
  message: string
}

export interface AutoscalePolicyFields {
  min_replicas: number
  max_replicas: number
  metric: AutoscaleMetric
  target: number
  scale_up_tolerance_pct: number
  scale_down_tolerance_pct: number
  scale_up_cooldown_secs: number
  scale_down_cooldown_secs: number
  stabilization_secs: number
  max_step_up: number
  max_step_down: number
}

export interface Autoscale extends AutoscalePolicyFields {
  app: string
  effective_max: number
  current: number
  desired: number
  value: number
  value_known: boolean
  blocked_by: string
  flaps_1h: number
  flapping: boolean
  last_scale_up?: string
  last_scale_down?: string
  evaluated_at?: string
  condition: AutoscaleCondition
}

export interface AutoscaleEvent {
  id: number
  kind: string
  current: number
  desired: number
  value: number
  reason: string
  blocked_by: string
  detail: string
  at: string
}

export const autoscaleKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'autoscale'] as const,
  history: (appName: string) =>
    [...appKeys.detail(appName), 'autoscale-history'] as const,
}

function path(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/autoscale${suffix}`
}

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

/** A null result means the app has no policy yet, which is not an error. */
async function getAutoscale(appName: string): Promise<Autoscale | null> {
  const res = await fetch(path(appName))
  if (res.status === 404) return null
  if (!res.ok) return fail(res, 'autoscale request')
  return (await res.json()) as Autoscale
}

async function putAutoscale(
  appName: string,
  body: AutoscalePolicyFields,
): Promise<Autoscale> {
  const res = await fetch(path(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) return fail(res, 'autoscale save')
  return (await res.json()) as Autoscale
}

async function deleteAutoscale(appName: string): Promise<void> {
  const res = await fetch(path(appName), { method: 'DELETE' })
  if (!res.ok) return fail(res, 'autoscale removal')
}

async function getHistory(appName: string): Promise<AutoscaleEvent[]> {
  const res = await fetch(path(appName, '/history?limit=20'))
  if (!res.ok) return fail(res, 'autoscale history')
  const body = (await res.json()) as { events: AutoscaleEvent[] }
  return body.events
}

export function autoscaleQueryOptions(appName: string) {
  return queryOptions({
    queryKey: autoscaleKeys.detail(appName),
    queryFn: () => getAutoscale(appName),
    refetchInterval: 30_000,
    retry: false,
  })
}

export function autoscaleHistoryQueryOptions(appName: string) {
  return queryOptions({
    queryKey: autoscaleKeys.history(appName),
    queryFn: () => getHistory(appName),
    refetchInterval: 30_000,
    retry: false,
  })
}

export function useAutoscaleActions(appName: string) {
  const queryClient = useQueryClient()
  const refresh = () => {
    void queryClient.invalidateQueries({
      queryKey: autoscaleKeys.history(appName),
    })
    void queryClient.invalidateQueries({ queryKey: appKeys.detail(appName) })
  }
  const save = useMutation<Autoscale, ApiError, AutoscalePolicyFields>({
    mutationFn: (body) => putAutoscale(appName, body),
    onSuccess: (policy) => {
      queryClient.setQueryData(autoscaleKeys.detail(appName), policy)
      refresh()
    },
  })
  const clear = useMutation<void, ApiError, void>({
    mutationFn: () => deleteAutoscale(appName),
    onSuccess: () => {
      queryClient.setQueryData(autoscaleKeys.detail(appName), null)
      refresh()
    },
  })
  return { save, clear }
}
