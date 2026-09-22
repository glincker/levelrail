// Query-key factory and fetchers for the WireGuard mesh status/rotation
// routes, mirroring queries/nodes.ts exactly: no ad hoc key arrays inline
// in components.
//
// GET /api/v1/mesh returns 501 when APP_MESH_ENABLED is unset (the
// default) on the target control plane, per internal/api/mesh.go's own
// doc comment. useMeshStatus treats that as a real, expected "nothing to
// show" state rather than a page-breaking error, the same
// graceful-degradation shape useNodePatchStatus already uses for
// telemetry not being configured.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { MeshStatusResource, RotateKeyResponse } from '../types/mesh'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nodeKeys } from './nodes'

export const meshKeys = {
  all: ['mesh'] as const,
  status: () => [...meshKeys.all, 'status'] as const,
}

export async function fetchMeshStatus(): Promise<MeshStatusResource> {
  const res = await fetch('/api/v1/mesh')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch mesh status failed: ${res.status}`),
    )
  }
  return (await res.json()) as MeshStatusResource
}

export function meshStatusQueryOptions() {
  return queryOptions({
    queryKey: meshKeys.status(),
    queryFn: fetchMeshStatus,
  })
}

// Not suspense: a control plane with mesh networking disabled (the
// default) makes this route a normal, common 501, not an exceptional
// one, so callers render their own "mesh not enabled" state off
// isError/error instead of the route crashing.
export function useMeshStatus() {
  return useQuery({ ...meshStatusQueryOptions(), retry: false })
}

// POST /api/v1/nodes/{id}/mesh/rotate-key (internal/api/mesh.go's
// handleRotateNodeMeshKey): rotates id's WireGuard key immediately.
// Invalidates mesh status (the new public key and a fresh, unconfirmed
// rotation record) and the node detail/list queries (nothing on
// nodeResource itself changes today, but a future node-scoped mesh
// summary would live there, matching useDrainNode's own
// broader-than-strictly-necessary invalidation for the same reason:
// staying correct as the read side grows costs one extra invalidate
// call today).
export async function rotateNodeMeshKey(
  id: string,
): Promise<RotateKeyResponse> {
  const res = await fetch(
    `/api/v1/nodes/${encodeURIComponent(id)}/mesh/rotate-key`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `rotate mesh key failed: ${res.status}`),
    )
  }
  return (await res.json()) as RotateKeyResponse
}

export function useRotateNodeMeshKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: rotateNodeMeshKey,
    onSuccess: (_result, id) => {
      void queryClient.invalidateQueries({ queryKey: meshKeys.status() })
      void queryClient.invalidateQueries({ queryKey: nodeKeys.detail(id) })
    },
  })
}
