import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PrivateNetwork {
  id: string
  name: string
  project_id?: string
  default_deny: boolean
  members: string[]
  created_at: string
}

export interface PrivateNetworkRule {
  id: string
  from: string
  to: string
  note?: string
  created_at: string
}

export interface PrivateNetworkList {
  networks: PrivateNetwork[]
  rules: PrivateNetworkRule[]
}

export interface PrivateNetworkMapApp {
  id: string
  name: string
  project_id?: string
  node_id?: string
  networks: string[]
}

export type EdgeObserved = 'connected' | 'isolated' | 'unknown'
export type EdgeFix = 'allow' | 'reapply' | ''

export interface PrivateNetworkMapEdge {
  a: string
  b: string
  allowed: boolean
  via?: 'network' | 'rule'
  via_name?: string
  rule_id?: string
  cross_node: boolean
  observed: EdgeObserved
  fix?: EdgeFix
}

export interface PrivateNetworkMap {
  apps: PrivateNetworkMapApp[]
  edges: PrivateNetworkMapEdge[]
}

export interface PrivateNetworkInput {
  name: string
  default_deny: boolean
  members: string[]
}

const BASE = '/api/v1/private-networks'

export const privateNetworkKeys = {
  all: ['private-networks'] as const,
  list: ['private-networks', 'list'] as const,
  map: ['private-networks', 'map'] as const,
}

async function send<T>(
  url: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${url} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function privateNetworkListOptions() {
  return queryOptions({
    queryKey: privateNetworkKeys.list,
    queryFn: () => send<PrivateNetworkList>(BASE, 'GET'),
  })
}

export function privateNetworkMapOptions() {
  return queryOptions({
    queryKey: privateNetworkKeys.map,
    queryFn: () => send<PrivateNetworkMap>(`${BASE}/map`, 'GET'),
    refetchInterval: 15_000,
  })
}

export function usePrivateNetworks() {
  return useQuery(privateNetworkListOptions())
}

export function usePrivateNetworkMap() {
  return useQuery(privateNetworkMapOptions())
}

function useInvalidating<V, R>(fn: (v: V) => Promise<R>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: privateNetworkKeys.all }),
  })
}

export function useCreatePrivateNetwork() {
  return useInvalidating((v: PrivateNetworkInput) =>
    send<{ id: string }>(BASE, 'POST', v),
  )
}

export function useUpdatePrivateNetwork() {
  return useInvalidating(
    (v: { id: string } & Omit<PrivateNetworkInput, 'name'>) =>
      send<{ id: string }>(`${BASE}/${encodeURIComponent(v.id)}`, 'PUT', {
        default_deny: v.default_deny,
        members: v.members,
      }),
  )
}

export function useDeletePrivateNetwork() {
  return useInvalidating((id: string) =>
    send<{ deleted: boolean }>(`${BASE}/${encodeURIComponent(id)}`, 'DELETE'),
  )
}

export function useCreatePrivateNetworkRule() {
  return useInvalidating((v: { from: string; to: string; note?: string }) =>
    send<{ id: string }>(`${BASE}/rules`, 'POST', v),
  )
}

export function useDeletePrivateNetworkRule() {
  return useInvalidating((id: string) =>
    send<{ deleted: boolean }>(
      `${BASE}/rules/${encodeURIComponent(id)}`,
      'DELETE',
    ),
  )
}

export function useApplyPrivateNetworks() {
  return useInvalidating(() =>
    send<{ enforced: boolean }>(`${BASE}/apply`, 'POST'),
  )
}
