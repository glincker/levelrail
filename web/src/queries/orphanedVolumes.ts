// Query and mutation for GET /api/v1/system/volumes/orphaned and POST
// /api/v1/system/volumes/orphaned/cleanup (internal/api/volumes_orphaned.go):
// named Docker volumes this instance created that no current app,
// database, or storage attachment references any more. Kept in its own
// module, the same reasoning queries/systemPrune.ts already gives for
// staying separate from queries/systemStatus.ts: a genuinely different
// operation even though both concern the same system-maintenance area
// of the UI.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { systemStatusKeys } from './systemStatus'

// OrphanedVolume mirrors internal/api's orphanedVolumeResource exactly.
// size_bytes is absent when the volume driver didn't report a size,
// never a fabricated 0.
export interface OrphanedVolume {
  name: string
  size_bytes?: number
  created_at?: string
}

export interface CleanupOrphanedVolumesResult {
  removed: string[]
  reclaimed_bytes: number
  skipped?: string[]
  errors?: string[]
}

export const orphanedVolumesKeys = {
  all: ['orphaned-volumes'] as const,
}

async function fetchOrphanedVolumes(): Promise<OrphanedVolume[]> {
  const res = await fetch('/api/v1/system/volumes/orphaned')
  if (res.status === 501) {
    return []
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list orphaned volumes failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrphanedVolume[]
}

// useOrphanedVolumes is a plain (non-suspense) query: the General
// settings page's own loader only primes systemStatusQueryOptions/
// certificatesQueryOptions, so this degrades to a loading state on its
// own card rather than blocking the whole route, the same reasoning
// useCertificates already follows on that same page.
export function useOrphanedVolumes() {
  return useQuery({
    queryKey: orphanedVolumesKeys.all,
    queryFn: fetchOrphanedVolumes,
  })
}

async function cleanupOrphanedVolumes(
  names: string[],
): Promise<CleanupOrphanedVolumesResult> {
  const res = await fetch('/api/v1/system/volumes/orphaned/cleanup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ names }),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Docker volume management requires a working Docker connection on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clean up failed: ${res.status}`),
    )
  }
  return (await res.json()) as CleanupOrphanedVolumesResult
}

// On success, invalidates both this list (removed/skipped volumes must
// disappear or stay depending on what actually happened) and system
// status (Docker disk usage numbers should reflect what was reclaimed),
// the same pattern useTriggerSystemPrune already establishes.
export function useCleanupOrphanedVolumes() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: cleanupOrphanedVolumes,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: orphanedVolumesKeys.all })
      void queryClient.invalidateQueries({ queryKey: systemStatusKeys.all })
    },
  })
}
