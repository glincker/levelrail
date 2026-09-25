// Query-key factory, fetchers and hooks for /api/v1/apps/{name}/loadbalancer
// (internal/api/loadbalancer.go): config, live upstream status and IaC export.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type LoadBalancerAlgorithm =
  'round_robin' | 'least_conn' | 'ip_hash' | 'uri_hash' | 'cookie' | 'weighted'

export type LoadBalancerExportFormat =
  'terraform' | 'cdk' | 'cloudformation' | 'caddy' | 'caddy-json'

export interface ActiveHealthConfig {
  path: string
  interval?: string
  timeout?: string
  passes?: number
  fails?: number
  expect_status?: number
}

export interface PassiveHealthConfig {
  fail_duration?: string
  max_fails?: number
}

export interface RetriesConfig {
  count?: number
  try_duration?: string
  try_interval?: string
}

export interface RateLimitConfig {
  rps: number
  burst?: number
}

export interface UpstreamTlsConfig {
  insecure_skip_verify?: boolean
  server_name?: string
}

// Mirrors internal/loadbalancer.Config exactly.
export interface LoadBalancerConfig {
  algorithm?: LoadBalancerAlgorithm
  cookie_name?: string
  weights?: number[]
  active_health?: ActiveHealthConfig
  passive_health?: PassiveHealthConfig
  retries?: RetriesConfig
  slow_start?: string
  drain_timeout?: string
  request_timeout?: string
  rate_limit?: RateLimitConfig
  upstream_tls?: UpstreamTlsConfig
}

export interface LoadBalancerResource {
  app_name: string
  configured: boolean
  config?: LoadBalancerConfig
  algorithms: LoadBalancerAlgorithm[]
  export_formats: LoadBalancerExportFormat[]
}

export type UpstreamState = 'healthy' | 'unhealthy' | 'draining' | 'unknown'

export interface UpstreamStatus {
  id: string
  dial: string
  node_id?: string
  replica: number
  weight: number
  state: UpstreamState
  healthy: boolean
  active_connections: number
  fails: number
  last_check?: string
  latency_ms?: number
  reason?: string
}

export interface LoadBalancerStatus {
  service: string
  algorithm: LoadBalancerAlgorithm
  ready: boolean
  reason: string
  message: string
  observed_at: string
  upstreams: UpstreamStatus[]
}

export interface LoadBalancerArtifact {
  format: LoadBalancerExportFormat
  filename: string
  content_type: string
  body: string
  warnings?: string[]
}

export const appLoadBalancerKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'loadbalancer'] as const,
  status: (appName: string) =>
    [...appKeys.detail(appName), 'loadbalancer', 'status'] as const,
  export: (appName: string, format: LoadBalancerExportFormat) =>
    [...appKeys.detail(appName), 'loadbalancer', 'export', format] as const,
}

function lbPath(appName: string, suffix = ''): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/loadbalancer${suffix}`
}

async function requestJson<T>(
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

export function appLoadBalancerQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appLoadBalancerKeys.detail(appName),
    queryFn: () =>
      requestJson<LoadBalancerResource>(
        lbPath(appName),
        undefined,
        'fetch load balancer',
      ),
  })
}

export function useAppLoadBalancer(appName: string) {
  return useQuery(appLoadBalancerQueryOptions(appName))
}

// Polled while mounted: status is the live view of an operator's traffic
// pool, so a stale table is worse than one extra small request.
export function useAppLoadBalancerStatus(appName: string, enabled: boolean) {
  return useQuery({
    queryKey: appLoadBalancerKeys.status(appName),
    queryFn: () =>
      requestJson<LoadBalancerStatus>(
        lbPath(appName, '/status'),
        undefined,
        'fetch load balancer status',
      ),
    enabled,
    refetchInterval: 5000,
  })
}

export function useSetAppLoadBalancer(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<LoadBalancerResource, ApiError, LoadBalancerConfig>({
    mutationFn: (config) =>
      requestJson<LoadBalancerResource>(
        lbPath(appName),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(config),
        },
        'save load balancer',
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(appLoadBalancerKeys.detail(appName), updated)
      void queryClient.invalidateQueries({
        queryKey: appLoadBalancerKeys.status(appName),
      })
    },
  })
}

export function useClearAppLoadBalancer(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(lbPath(appName), { method: 'DELETE' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `clear load balancer failed: ${res.status}`,
          ),
        )
      }
    },
    onSuccess: () => {
      queryClient.setQueryData(appLoadBalancerKeys.detail(appName), {
        app_name: appName,
        configured: false,
        algorithms: [],
        export_formats: [],
      } satisfies LoadBalancerResource)
      queryClient.removeQueries({
        queryKey: appLoadBalancerKeys.status(appName),
      })
    },
  })
}

export function useLoadBalancerExport(
  appName: string,
  format: LoadBalancerExportFormat,
  enabled: boolean,
) {
  return useQuery({
    queryKey: appLoadBalancerKeys.export(appName, format),
    queryFn: () =>
      requestJson<LoadBalancerArtifact>(
        lbPath(appName, `/export?format=${encodeURIComponent(format)}`),
        undefined,
        'export load balancer',
      ),
    enabled,
    staleTime: 0,
  })
}
