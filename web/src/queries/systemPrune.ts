// Mutation for POST /api/v1/system/prune (internal/api/system_prune.go's
// handleSystemPrune): the "clean up now" action on the General settings
// page. Kept in its own module rather than folded into systemStatus.ts,
// the same reasoning queries/deploys.ts gives for staying separate from
// queries/apps.ts: a genuinely different operation (a destructive
// action, not a read) even though both concern the same system-status
// area of the UI.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { systemStatusKeys } from './systemStatus'

// SystemPruneResult mirrors internal/api/system_prune.go's
// systemPruneResponse exactly: what was actually removed, per resource
// kind, plus any per-stage error that didn't stop the rest.
export interface SystemPruneResult {
  containers_removed: string[]
  containers_reclaimed_bytes: number
  images_removed: string[]
  images_reclaimed_bytes: number
  volumes_removed: string[]
  volumes_reclaimed_bytes: number
  build_cache_reclaimed_bytes: number
  errors?: string[]
}

// 501 means no Docker connection was established at startup
// (WithDockerPruner never applied): a clear, distinct message rather
// than the generic readErrorMessage fallback, the same shape
// triggerBackup's own 501 branch already establishes for a missing
// master key.
export async function triggerSystemPrune(): Promise<SystemPruneResult> {
  const res = await fetch('/api/v1/system/prune', { method: 'POST' })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Docker cleanup requires a working Docker connection on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clean up failed: ${res.status}`),
    )
  }
  return (await res.json()) as SystemPruneResult
}

// On success, invalidates the system status query so the disk-usage
// numbers on the General settings page reflect what was actually
// reclaimed on the next read, rather than this mutation trying to
// compute the new totals itself client-side.
export function useTriggerSystemPrune() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: triggerSystemPrune,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: systemStatusKeys.all })
    },
  })
}
