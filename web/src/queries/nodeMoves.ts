// Verified moves, node drains, labels, placement and mesh connectivity:
// the multi-node routes under /api/v1 (nodes, node-moves, node-drains,
// apps|databases/{name}/move|placement, mesh/connectivity).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  MeshConnectivity,
  NodeDrain,
  NodeLabels,
  NodeMove,
  NodeMoveKind,
  Placement,
} from '../types/nodeMoves'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'
import { nodeKeys } from './nodes'

const POLL_MS = 2_000

const JSON_HEADERS = { 'Content-Type': 'application/json' }

export const nodeMoveKeys = {
  all: ['node-moves'] as const,
  detail: (id: string) => [...nodeMoveKeys.all, 'detail', id] as const,
  forResource: (kind: NodeMoveKind, name: string) =>
    [...nodeMoveKeys.all, 'resource', kind, name] as const,
  drain: (id: string) => ['node-drains', id] as const,
  drains: (nodeId: string) => [...nodeKeys.detail(nodeId), 'drains'] as const,
  labels: (nodeId: string) => [...nodeKeys.detail(nodeId), 'labels'] as const,
  placement: (kind: NodeMoveKind, name: string) =>
    ['placement', kind, name] as const,
  connectivity: () => ['mesh', 'connectivity'] as const,
}

function resourcePath(kind: NodeMoveKind, name: string): string {
  const collection = kind === 'app' ? 'apps' : 'databases'
  return `/api/v1/${collection}/${encodeURIComponent(name)}`
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export async function startNodeMove(
  kind: NodeMoveKind,
  name: string,
  nodeId: string,
): Promise<NodeMove> {
  const res = await fetch(`${resourcePath(kind, name)}/move`, {
    method: 'POST',
    headers: JSON_HEADERS,
    body: JSON.stringify({ node_id: nodeId }),
  })
  if (!res.ok) {
    throw await failure(res, 'move')
  }
  return (await res.json()) as NodeMove
}

export async function fetchNodeMove(id: string): Promise<NodeMove> {
  const res = await fetch(`/api/v1/node-moves/${encodeURIComponent(id)}`)
  if (!res.ok) {
    throw await failure(res, 'fetch move')
  }
  return (await res.json()) as NodeMove
}

export function useStartNodeMove(kind: NodeMoveKind, name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (nodeId: string) => startNodeMove(kind, name, nodeId),
    onSuccess: (move) => {
      queryClient.setQueryData(nodeMoveKeys.detail(move.id), move)
    },
  })
}

// Polls every 2s while the move is running, then invalidates whatever the
// move changed so placement shown elsewhere is fresh.
export function useNodeMove(id: string | null) {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: nodeMoveKeys.detail(id ?? ''),
    queryFn: async () => {
      if (!id) {
        throw new Error('no move id')
      }
      const move = await fetchNodeMove(id)
      if (move.status !== 'running') {
        void queryClient.invalidateQueries({ queryKey: appKeys.all })
        void queryClient.invalidateQueries({ queryKey: databaseKeys.all })
        void queryClient.invalidateQueries({ queryKey: nodeKeys.all })
        void queryClient.invalidateQueries({
          queryKey: ['placement', move.kind, move.resource],
        })
      }
      return move
    },
    enabled: id !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? POLL_MS : false,
  })
}

export async function startNodeDrain(
  nodeId: string,
  targetNodeId: string,
): Promise<NodeDrain> {
  const params = new URLSearchParams()
  if (targetNodeId) {
    params.set('target_node_id', targetNodeId)
  }
  const query = params.toString()
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/drain${query ? `?${query}` : ''}`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw await failure(res, 'drain node')
  }
  return (await res.json()) as NodeDrain
}

export async function fetchNodeDrain(id: string): Promise<NodeDrain> {
  const res = await fetch(`/api/v1/node-drains/${encodeURIComponent(id)}`)
  if (!res.ok) {
    throw await failure(res, 'fetch drain')
  }
  return (await res.json()) as NodeDrain
}

export async function fetchNodeDrains(nodeId: string): Promise<NodeDrain[]> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(nodeId)}/drains`)
  if (!res.ok) {
    throw await failure(res, 'fetch drains')
  }
  return ((await res.json()) as NodeDrain[] | null) ?? []
}

export function useStartNodeDrain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, targetNodeId }: { id: string; targetNodeId: string }) =>
      startNodeDrain(id, targetNodeId),
    onSuccess: (drain, { id }) => {
      queryClient.setQueryData(nodeMoveKeys.drain(drain.id), drain)
      void queryClient.invalidateQueries({ queryKey: nodeKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: nodeKeys.list() })
    },
  })
}

function invalidateAfterDrain(
  queryClient: ReturnType<typeof useQueryClient>,
  drain: NodeDrain,
) {
  void queryClient.invalidateQueries({ queryKey: nodeKeys.all })
  void queryClient.invalidateQueries({ queryKey: appKeys.all })
  void queryClient.invalidateQueries({ queryKey: databaseKeys.all })
  void queryClient.invalidateQueries({
    queryKey: nodeMoveKeys.drains(drain.node_id),
  })
}

export function useNodeDrain(id: string | null) {
  const queryClient = useQueryClient()
  return useQuery({
    queryKey: nodeMoveKeys.drain(id ?? ''),
    queryFn: async () => {
      if (!id) {
        throw new Error('no drain id')
      }
      const drain = await fetchNodeDrain(id)
      if (drain.status !== 'running') {
        invalidateAfterDrain(queryClient, drain)
      }
      return drain
    },
    enabled: id !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? POLL_MS : false,
  })
}

export function useNodeDrains(nodeId: string) {
  return useQuery({
    queryKey: nodeMoveKeys.drains(nodeId),
    queryFn: () => fetchNodeDrains(nodeId),
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((d) => d.status === 'running') ? POLL_MS : false,
  })
}

export async function fetchNodeLabels(nodeId: string): Promise<NodeLabels> {
  const res = await fetch(`/api/v1/nodes/${encodeURIComponent(nodeId)}/labels`)
  if (!res.ok) {
    throw await failure(res, 'fetch node labels')
  }
  const body = (await res.json()) as { labels?: NodeLabels | null }
  return body.labels ?? {}
}

export async function putNodeLabels(
  nodeId: string,
  labels: NodeLabels,
): Promise<NodeLabels> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(nodeId)}/labels`,
    {
      method: 'PUT',
      headers: JSON_HEADERS,
      body: JSON.stringify({ labels }),
    },
  )
  if (!res.ok) {
    throw await failure(res, 'save node labels')
  }
  const body = (await res.json()) as { labels?: NodeLabels | null }
  return body.labels ?? {}
}

export function useNodeLabels(nodeId: string) {
  return useQuery({
    queryKey: nodeMoveKeys.labels(nodeId),
    queryFn: () => fetchNodeLabels(nodeId),
    retry: false,
  })
}

export function useSetNodeLabels(nodeId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (labels: NodeLabels) => putNodeLabels(nodeId, labels),
    onSuccess: (labels) => {
      queryClient.setQueryData(nodeMoveKeys.labels(nodeId), labels)
      void queryClient.invalidateQueries({ queryKey: ['placement'] })
    },
  })
}

export async function fetchPlacement(
  kind: NodeMoveKind,
  name: string,
): Promise<Placement> {
  const res = await fetch(`${resourcePath(kind, name)}/placement`)
  if (!res.ok) {
    throw await failure(res, 'fetch placement')
  }
  const body = (await res.json()) as Placement
  return {
    ...body,
    node_selector: body.node_selector ?? {},
    eligible_node_ids: body.eligible_node_ids ?? [],
    rejected: body.rejected ?? [],
  }
}

export async function putPlacement(
  kind: NodeMoveKind,
  name: string,
  nodeSelector: NodeLabels,
): Promise<Placement> {
  const res = await fetch(`${resourcePath(kind, name)}/placement`, {
    method: 'PUT',
    headers: JSON_HEADERS,
    body: JSON.stringify({ node_selector: nodeSelector }),
  })
  if (!res.ok) {
    throw await failure(res, 'save placement')
  }
  return (await res.json()) as Placement
}

export function usePlacement(kind: NodeMoveKind, name: string, enabled = true) {
  return useQuery({
    queryKey: nodeMoveKeys.placement(kind, name),
    queryFn: () => fetchPlacement(kind, name),
    enabled,
    retry: false,
  })
}

export function useSetPlacement(kind: NodeMoveKind, name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (nodeSelector: NodeLabels) =>
      putPlacement(kind, name, nodeSelector),
    onSuccess: (placement) => {
      queryClient.setQueryData(nodeMoveKeys.placement(kind, name), placement)
    },
  })
}

export async function fetchMeshConnectivity(): Promise<MeshConnectivity> {
  const res = await fetch('/api/v1/mesh/connectivity')
  if (!res.ok) {
    throw await failure(res, 'fetch mesh connectivity')
  }
  return (await res.json()) as MeshConnectivity
}

export function meshConnectivityQueryOptions() {
  return queryOptions({
    queryKey: nodeMoveKeys.connectivity(),
    queryFn: fetchMeshConnectivity,
    retry: false,
  })
}

// 501 means mesh networking is off: callers render an empty state.
export function useMeshConnectivity() {
  return useQuery(meshConnectivityQueryOptions())
}
