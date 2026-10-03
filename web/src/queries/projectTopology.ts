// Query-key factory and fetcher for GET /api/v1/projects/{id}/topology
// (internal/api/topology.go), mirroring queries/networkTopology.ts's own
// shape: AbilityRead-gated, one request, no mutations.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { ProjectTopologyGraph } from '../types/projectTopology'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const projectTopologyKeys = {
  all: ['project-topology'] as const,
  detail: (projectId: string) =>
    [...projectTopologyKeys.all, projectId] as const,
}

export async function fetchProjectTopology(
  projectId: string,
): Promise<ProjectTopologyGraph> {
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(projectId)}/topology`,
  )
  if (res.status === 404) {
    throw new ApiError(404, `project not found: ${projectId}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch project topology failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ProjectTopologyGraph
}

export function projectTopologyQueryOptions(projectId: string) {
  return queryOptions({
    queryKey: projectTopologyKeys.detail(projectId),
    queryFn: () => fetchProjectTopology(projectId),
  })
}

export function useProjectTopology(projectId: string) {
  return useSuspenseQuery(projectTopologyQueryOptions(projectId))
}
