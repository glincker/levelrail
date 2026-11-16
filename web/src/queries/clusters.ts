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
  DeployK8sInput,
  K8sDeployment,
  K8sDeploymentStatus,
} from '../types/clusters'

export const clusterKeys = {
  all: ['clusters'] as const,
  list: () => [...clusterKeys.all, 'list'] as const,
  nodes: (name: string) => [...clusterKeys.all, 'nodes', name] as const,
  namespaces: (name: string) =>
    [...clusterKeys.all, 'namespaces', name] as const,
  deployments: (name: string) =>
    [...clusterKeys.all, 'deployments', name] as const,
  logs: (name: string, app: string, pod: string, tail: number) =>
    [...clusterKeys.all, 'logs', name, app, pod, tail] as const,
  status: (name: string, app: string) =>
    [...clusterKeys.all, 'status', name, app] as const,
}

// Status polls only until the app is ready, then stops.
const STATUS_POLL_MS = Number(import.meta.env.VITE_K8S_STATUS_POLL_MS) || 4000

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

const appBase = (name: string, app: string) =>
  `${base(name)}/deployments/${encodeURIComponent(app)}`

export function useClusterDeployments(name: string) {
  return useQuery({
    queryKey: clusterKeys.deployments(name),
    retry: false,
    queryFn: async () =>
      (
        await request<{ deployments: K8sDeployment[] }>(
          `${base(name)}/deployments`,
          undefined,
          'list deployments',
        )
      ).deployments,
  })
}

export function useDeploymentStatus(name: string, app: string) {
  return useQuery({
    queryKey: clusterKeys.status(name, app),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.ready === true ? false : STATUS_POLL_MS,
    queryFn: () =>
      request<K8sDeploymentStatus>(
        appBase(name, app),
        undefined,
        'deployment status',
      ),
  })
}

export function useDeployApp(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (input: DeployK8sInput) =>
      request<{ app: string }>(
        `${base(name)}/deployments`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'deploy app',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: clusterKeys.deployments(name),
      }),
  })
}

export interface DeploymentLogs {
  text: string
  truncated: boolean
}

export function useDeploymentLogs(
  name: string,
  app: string,
  pod: string,
  tail: number,
  enabled: boolean,
) {
  return useQuery({
    queryKey: clusterKeys.logs(name, app, pod, tail),
    enabled,
    retry: false,
    staleTime: 0,
    queryFn: async (): Promise<DeploymentLogs> => {
      const qs = new URLSearchParams({ tail: String(tail) })
      if (pod) qs.set('pod', pod)
      const res = await fetch(`${appBase(name, app)}/logs?${qs.toString()}`)
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `logs failed: ${String(res.status)}`),
        )
      }
      return {
        text: await res.text(),
        truncated: res.headers.get('X-Levelrail-Truncated') === 'true',
      }
    },
  })
}

export function useRemoveDeployment(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (app: string) =>
      request<{ app: string }>(
        appBase(name, app),
        { method: 'DELETE' },
        'remove deployment',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: clusterKeys.deployments(name),
      }),
  })
}
