// GET/PUT /api/v1/nodes/{id}/thresholds (internal/api/node_thresholds.go).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { nodeKeys } from './nodes'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface NodeThresholdValues {
  cpu_percent: number | null
  memory_percent: number | null
  disk_percent: number | null
}

export interface NodeThresholds {
  node_id: string
  defaults: NodeThresholdValues
  override: NodeThresholdValues
  effective: NodeThresholdValues
  sustained_intervals: number
}

export const nodeThresholdKeys = {
  detail: (nodeId: string) =>
    [...nodeKeys.detail(nodeId), 'thresholds'] as const,
}

function thresholdsPath(nodeId: string) {
  return `/api/v1/nodes/${encodeURIComponent(nodeId)}/thresholds`
}

async function readThresholds(res: Response): Promise<NodeThresholds> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `node thresholds failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeThresholds
}

export async function fetchNodeThresholds(nodeId: string) {
  return readThresholds(await fetch(thresholdsPath(nodeId)))
}

export function useNodeThresholds(nodeId: string) {
  return useQuery({
    queryKey: nodeThresholdKeys.detail(nodeId),
    queryFn: () => fetchNodeThresholds(nodeId),
    retry: false,
  })
}

export function useSetNodeThresholds(nodeId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: async (override: NodeThresholdValues) =>
      readThresholds(
        await fetch(thresholdsPath(nodeId), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(override),
        }),
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(nodeThresholdKeys.detail(nodeId), updated)
    },
  })
}
