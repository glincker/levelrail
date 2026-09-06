// Query-key factory and fetcher for GET /api/v1/system/containers
// (internal/api/containers.go's handleListContainers): every container
// Docker knows about on this node, whether or not Levelrail manages it.
// Mirrors systemStatus.ts's own read-only query shape.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

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
