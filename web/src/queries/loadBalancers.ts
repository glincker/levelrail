// Query-key factory and fetcher for GET /api/v1/loadbalancers
// (internal/api/loadbalancer_list.go): the cross-app load balancer overview.

import { keepPreviousData, queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { LoadBalancerAlgorithm } from './appLoadBalancer'

export type LoadBalancerOverviewState = 'balancing' | 'degraded' | 'none'

export const LB_STATE_LABEL: Record<LoadBalancerOverviewState, string> = {
  balancing: 'Balancing',
  degraded: 'Degraded',
  none: 'No upstreams',
}

export interface LoadBalancerSummary {
  app: string
  service: string
  algorithm: LoadBalancerAlgorithm
  state: LoadBalancerOverviewState
  upstreams_total: number
  upstreams_healthy: number
  reason?: string
  last_check?: string
  config_updated_at: string
  active_health_check: boolean
}

export interface LoadBalancerList {
  items: LoadBalancerSummary[]
  total: number
  limit: number
  offset: number
}

export interface LoadBalancerListParams {
  search: string
  state: LoadBalancerOverviewState | ''
}

export const OVERVIEW_PAGE_LIMIT = 500

export const loadBalancerListKeys = {
  all: ['loadbalancers'] as const,
  list: (params: LoadBalancerListParams) =>
    [...loadBalancerListKeys.all, 'list', params.search, params.state] as const,
}

export async function fetchLoadBalancers(
  params: LoadBalancerListParams,
): Promise<LoadBalancerList> {
  const q = new URLSearchParams({ limit: String(OVERVIEW_PAGE_LIMIT) })
  if (params.search) q.set('q', params.search)
  if (params.state) q.set('state', params.state)
  const res = await fetch(`/api/v1/loadbalancers?${q.toString()}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch load balancers failed: ${res.status}`),
    )
  }
  return (await res.json()) as LoadBalancerList
}

export function loadBalancerListQueryOptions(params: LoadBalancerListParams) {
  return queryOptions({
    queryKey: loadBalancerListKeys.list(params),
    queryFn: () => fetchLoadBalancers(params),
    refetchInterval: 5000,
    placeholderData: keepPreviousData,
  })
}

// Polled while mounted so upstream health stays live, the same reasoning
// as useAppLoadBalancerStatus.
export function useLoadBalancerList(params: LoadBalancerListParams) {
  return useQuery(loadBalancerListQueryOptions(params))
}
