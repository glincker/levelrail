// Query-key factory and fetcher for GET /api/v1/system/containers
// (internal/api/containers.go's handleListContainers): every container
// Docker knows about on this node, whether or not Levelrail manages it.
// Mirrors systemStatus.ts's own read-only query shape. Also carries the
// orphaned-container mutations (internal/api/containers_orphaned.go):
// stop, remove, and claim act only on a container this control plane
// doesn't manage, re-confirmed server-side on every call.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import type { AppDetail } from '../types/appDetail'

export const containerKeys = {
  all: ['system-containers'] as const,
}

export interface ContainerPort {
  container_port: number
  host_port: number
  protocol: string
}

// ContainerResource mirrors internal/api/containers.go's
// containerResource exactly.
export interface ContainerResource {
  name: string
  image: string
  running: boolean
  ports: ContainerPort[]
  managed: boolean
}

// 501 means no ContainerLister was wired up at startup (WithContainerLister
// never applied, e.g. no Docker connection): a clear, distinct message
// rather than the generic readErrorMessage fallback, the same shape
// triggerSystemPrune's own 501 branch establishes for a missing Docker
// connection.
export async function fetchContainers(): Promise<ContainerResource[]> {
  const res = await fetch('/api/v1/system/containers')
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Container listing requires a working Docker connection on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list containers failed: ${res.status}`),
    )
  }
  return (await res.json()) as ContainerResource[]
}

// A moderate staleTime, the same reasoning systemStatusQueryOptions
// gives: an operator visiting this page occasionally doesn't need
// live container tracking, a fresh read on each navigation is enough.
export function containersQueryOptions() {
  return queryOptions({
    queryKey: containerKeys.all,
    queryFn: fetchContainers,
    staleTime: 30_000,
  })
}

// Not suspense, and not prefetched in the route's own loader: this
// endpoint can legitimately 501 (no Docker connection), and the route
// shows that as an inline "not configured" state rather than crashing
// into a route-level error boundary, the same reasoning
// useSystemStatusOptional's own doc comment gives for an identical
// optional-dependency shape.
export function useContainers() {
  return useQuery({ ...containersQueryOptions(), retry: false })
}

// stop/remove both hit the same shape of route
// (POST /api/v1/system/containers/{name}/{action}), 501 when no
// OrphanedContainerManager is wired up, 409 if the container turns out
// to be Levelrail-managed (the server re-checks this itself, never
// trusting the client).
async function postOrphanedContainerAction(
  name: string,
  action: 'stop' | 'remove',
): Promise<void> {
  const res = await fetch(
    `/api/v1/system/containers/${encodeURIComponent(name)}/${action}`,
    { method: 'POST' },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Container management requires a working Docker connection on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${action} container failed: ${res.status}`),
    )
  }
}

export function useStopOrphanedContainer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => postOrphanedContainerAction(name, 'stop'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: containerKeys.all })
    },
  })
}

export function useRemoveOrphanedContainer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => postOrphanedContainerAction(name, 'remove'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: containerKeys.all })
    },
  })
}

// claimOrphanedContainer creates a real app from an orphaned container's
// own image (internal/api/containers_orphaned.go's
// handleClaimOrphanedContainer): build.type: image, not an adoption of
// the container's live state. name, if given, overrides the server's
// own derived-from-container-name default.
async function claimOrphanedContainer(
  containerName: string,
  name?: string,
): Promise<AppDetail> {
  const res = await fetch(
    `/api/v1/system/containers/${encodeURIComponent(containerName)}/claim`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(name ? { name } : {}),
    },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Container management requires a working Docker connection on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `claim container failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useClaimOrphanedContainer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      containerName,
      name,
    }: {
      containerName: string
      name?: string
    }) => claimOrphanedContainer(containerName, name),
    onSuccess: (created) => {
      queryClient.setQueryData(appKeys.detail(created.name), created)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: containerKeys.all })
    },
  })
}
