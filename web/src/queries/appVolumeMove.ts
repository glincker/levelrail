// Query-key factory and fetchers for "move this app to another node,
// taking its volumes with it" (internal/api/apps_move_with_volumes.go):
// POST/GET /api/v1/apps/{name}/move-with-volumes and .../moves/{id}.
// Mirrors queries/cloneRestore.ts's own shape: a mutation that kicks the
// operation off, plus a polling query for a caller to watch while it's
// still running.

import { queryOptions, useMutation, useQuery } from '@tanstack/react-query'
import type {
  AppVolumeMove,
  MoveAppWithVolumesRequest,
} from '../types/appVolumeMove'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appVolumeMoveKeys = {
  all: (serviceName: string) => ['apps', serviceName, 'moves'] as const,
  detail: (serviceName: string, moveId: string) =>
    [...appVolumeMoveKeys.all(serviceName), moveId] as const,
}

// Same polling cadence internal/api's own runAppVolumeMove step-writes
// aim for being visible within: a volume move is dominated by tar/untar
// I/O over the agent transport, no reason to expect a wildly different
// "how long until this finishes" answer than a plain volume backup.
const RUNNING_POLL_INTERVAL_MS = 1_000

// POST /api/v1/apps/{name}/move-with-volumes (handleMoveAppWithVolumes).
// Always returns an AppVolumeMove: status "succeeded" means it already
// finished (no volumes, or already on nodeId), "running" means the caller
// should poll GetAppVolumeMove (useAppVolumeMove below) until it isn't.
export async function moveAppWithVolumes(
  name: string,
  req: MoveAppWithVolumesRequest,
): Promise<AppVolumeMove> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/move-with-volumes`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Node-to-node volume moves are not configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `move app with volumes failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppVolumeMove
}

export function useMoveAppWithVolumes(name: string) {
  return useMutation({
    mutationFn: (req: MoveAppWithVolumesRequest) => moveAppWithVolumes(name, req),
  })
}

// GET /api/v1/apps/{name}/moves/{id} (handleGetAppVolumeMove): what the
// dialog polls while a triggered move is still "running".
export async function fetchAppVolumeMove(
  name: string,
  moveId: string,
): Promise<AppVolumeMove> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/moves/${encodeURIComponent(moveId)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app volume move failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppVolumeMove
}

export function appVolumeMoveQueryOptions(name: string, moveId: string) {
  return queryOptions({
    queryKey: appVolumeMoveKeys.detail(name, moveId),
    queryFn: () => fetchAppVolumeMove(name, moveId),
  })
}

// moveId null disables the query entirely: nothing to poll before a move
// has actually been triggered.
export function useAppVolumeMove(name: string, moveId: string | null) {
  return useQuery({
    ...appVolumeMoveQueryOptions(name, moveId ?? ''),
    enabled: moveId !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? RUNNING_POLL_INTERVAL_MS : false,
  })
}
