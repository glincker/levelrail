// Live operations for the load balancer page: check history, one-shot probe
// and per-upstream admin state. These endpoints may not exist on an older
// control plane, so callers treat 404 and 501 as "not supported".

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appLoadBalancerKeys, type UpstreamStatus } from './appLoadBalancer'

export type AdminState = 'active' | 'draining' | 'disabled'

export interface LiveUpstream extends UpstreamStatus {
  admin_state?: AdminState
  last_changed_at?: string
}

export interface CheckSample {
  at: string
  ok: boolean
  status_code?: number
  latency_ms?: number
  reason?: string
}

export interface StateTransition {
  at: string
  from: string
  to: string
  reason?: string
}

export interface SeriesPoint {
  at: string
  value: number
}

export interface UpstreamHistory {
  id: string
  dial: string
  admin_state: AdminState
  checks: CheckSample[]
  transitions: StateTransition[]
  series: {
    connections: SeriesPoint[]
    latency_ms: SeriesPoint[]
    fails: SeriesPoint[]
  }
}

export interface LoadBalancerHistory {
  upstreams: UpstreamHistory[]
}

export class RateLimitError extends ApiError {
  readonly retryAfter: number

  constructor(retryAfter: number) {
    super(429, 'Checks are rate limited')
    this.name = 'RateLimitError'
    this.retryAfter = retryAfter
  }
}

export interface CheckResponse {
  results: CheckResult[]
  note?: string
}

export interface CheckResult {
  id: string
  dial: string
  ok: boolean
  status_code?: number
  latency_ms?: number
  reason?: string
}

export const lbLiveKeys = {
  history: (appName: string) =>
    [...appLoadBalancerKeys.detail(appName), 'history'] as const,
}

export function isUnsupported(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 404 || error.status === 501)
  )
}

function base(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/loadbalancer`
}

async function send<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useLoadBalancerHistory(appName: string, enabled: boolean) {
  const query = useQuery({
    queryKey: lbLiveKeys.history(appName),
    queryFn: () =>
      send<LoadBalancerHistory>(
        `${base(appName)}/history?limit=60`,
        undefined,
        'fetch load balancer history',
      ),
    enabled,
    retry: false,
    refetchInterval: (q) => (isUnsupported(q.state.error) ? false : 10_000),
  })
  return {
    ...query,
    supported: !isUnsupported(query.error),
    byId: new Map((query.data?.upstreams ?? []).map((u) => [u.id, u])),
  }
}

export function useRunLoadBalancerCheck(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<CheckResponse, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(`${base(appName)}/check`, { method: 'POST' })
      if (res.status === 429) {
        const secs = Number.parseInt(res.headers?.get('Retry-After') ?? '', 10)
        throw new RateLimitError(Number.isFinite(secs) && secs > 0 ? secs : 5)
      }
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `run health check failed: ${res.status}`),
        )
      }
      return (await res.json()) as CheckResponse
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appLoadBalancerKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: lbLiveKeys.history(appName),
      })
    },
  })
}

export function useSetUpstreamAdminState(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    unknown,
    ApiError,
    { id: string; admin_state: AdminState }
  >({
    mutationFn: ({ id, admin_state }) =>
      send(
        `${base(appName)}/upstreams/${encodeURIComponent(id)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ admin_state }),
        },
        'update upstream',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appLoadBalancerKeys.status(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: lbLiveKeys.history(appName),
      })
    },
  })
}
