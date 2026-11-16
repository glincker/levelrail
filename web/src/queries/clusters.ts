// Fetchers and hooks for the Kubernetes cluster endpoints (internal/api/routes_k8s.go).
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  Cluster,
  ClusterList,
  ClusterNamespace,
  ClusterNode,
  ClusterTestResult,
  ConnectClusterInput,
} from '../types/clusters'

export const clusterKeys = {
  all: ['clusters'] as const,
  list: () => [...clusterKeys.all, 'list'] as const,
  nodes: (name: string) => [...clusterKeys.all, 'nodes', name] as const,
  namespaces: (name: string) =>
    [...clusterKeys.all, 'namespaces', name] as const,
}

const root = '/api/v1/clusters'
const base = (name: string) => `${root}/${encodeURIComponent(name)}`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json().catch(() => ({}))) as T
}

// 403 means the k8s.clusters_enabled runtime setting is off; 501 means no secrets store.
export function isClustersOff(error: unknown): boolean {
  return (
    error instanceof ApiError && (error.status === 403 || error.status === 501)
  )
}

export function clustersQueryOptions() {
  return queryOptions({
    queryKey: clusterKeys.list(),
    queryFn: async () =>
      (await request<ClusterList>(root, undefined, 'list clusters')).clusters,
    retry: false,
  })
}

export function useClusters() {
  return useQuery(clustersQueryOptions())
}

export function useClusterNodes(name: string, enabled: boolean) {
  return useQuery({
    queryKey: clusterKeys.nodes(name),
    enabled,
    retry: false,
    queryFn: async () =>
      (
        await request<{ nodes: ClusterNode[] }>(
          `${base(name)}/nodes`,
          undefined,
          'list nodes',
        )
      ).nodes,
  })
}

export function useClusterNamespaces(name: string, enabled: boolean) {
  return useQuery({
    queryKey: clusterKeys.namespaces(name),
    enabled,
    retry: false,
    queryFn: async () =>
      (
        await request<{ namespaces: ClusterNamespace[] }>(
          `${base(name)}/namespaces`,
          undefined,
          'list namespaces',
        )
      ).namespaces,
  })
}

export function useConnectCluster() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (input: ConnectClusterInput) =>
      request<Cluster>(
        root,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'connect cluster',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: clusterKeys.list() }),
  })
}

export function useTestCluster() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (name: string) =>
      request<ClusterTestResult>(
        `${base(name)}/test`,
        { method: 'POST' },
        'test cluster',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: clusterKeys.list() }),
  })
}

export function useDisconnectCluster() {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (name: string) =>
      request<undefined>(
        base(name),
        { method: 'DELETE' },
        'disconnect cluster',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: clusterKeys.all }),
  })
}
