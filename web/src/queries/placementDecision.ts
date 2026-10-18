// The resource-aware placement routes: GET /api/v1/nodes/allocations and
// GET /api/v1/{apps|databases}/{name}/placement/decision.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  NodeAllocations,
  PlacementDecisionView,
  ZonesView,
} from '../types/placementDecision'
import type { NodeMoveKind } from '../types/nodeMoves'

export const placementDecisionKeys = {
  allocations: () => ['nodes', 'allocations'] as const,
  zones: () => ['nodes', 'zones'] as const,
  decision: (kind: NodeMoveKind, name: string) =>
    ['placement', kind, name, 'decision'] as const,
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useNodeAllocations() {
  return useQuery({
    queryKey: placementDecisionKeys.allocations(),
    queryFn: () =>
      getJson<NodeAllocations>(
        '/api/v1/nodes/allocations',
        'fetch node allocations',
      ),
    retry: false,
  })
}

export function useZones() {
  return useQuery({
    queryKey: placementDecisionKeys.zones(),
    queryFn: () =>
      getJson<ZonesView>('/api/v1/nodes/zones', 'fetch node zones'),
    retry: false,
  })
}

export function usePlacementDecision(kind: NodeMoveKind, name: string) {
  return useQuery({
    queryKey: placementDecisionKeys.decision(kind, name),
    queryFn: async () => {
      const collection = kind === 'app' ? 'apps' : 'databases'
      const view = await getJson<PlacementDecisionView>(
        `/api/v1/${collection}/${encodeURIComponent(name)}/placement/decision`,
        'fetch placement decision',
      )
      return {
        ...view,
        preview: { ...view.preview, considered: view.preview.considered ?? [] },
        recorded: view.recorded
          ? {
              ...view.recorded,
              decision: {
                ...view.recorded.decision,
                considered: view.recorded.decision.considered ?? [],
              },
            }
          : null,
      }
    },
    retry: false,
  })
}
