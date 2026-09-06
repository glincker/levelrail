// Mutation for POST /api/v1/system/master-key/rotate
// (internal/api/master_key_rotation.go's handleRotateMasterKey): the
// "rotate master key" action on the General settings page. Kept in its
// own module rather than folded into systemStatus.ts, the same
// reasoning systemPrune.ts's own comment gives for a genuinely
// different operation (a destructive-if-mishandled write, not a read)
// even though both concern the same settings area.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// RotateMasterKeyResult mirrors internal/api/master_key_rotation.go's
// rotateMasterKeyResponse exactly.
export interface RotateMasterKeyResult {
  rotatedAt: string
  // persistedToFile is true once the new key was also written to disk:
  // a restart of this control plane picks it up automatically. False
  // when the master key is env-sourced (APP_MASTER_KEY) or the file
  // write itself failed, either way the operator has a required
  // follow-up (update APP_MASTER_KEY, or fix the file write) before the
  // next restart, explained in `warning`.
  persistedToFile: boolean
  warning?: string
}

// 501 means no master key was loaded at startup (no APP_MASTER_KEY, no
// key file): a clear, distinct message rather than the generic
// readErrorMessage fallback, the same shape triggerSystemPrune's own
// 501 branch establishes for a missing Docker connection.
export async function rotateMasterKey(
  newMasterKey: string,
): Promise<RotateMasterKeyResult> {
  const res = await fetch('/api/v1/system/master-key/rotate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ newMasterKey }),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Master key rotation requires a master key already loaded on this control plane (APP_MASTER_KEY or a key file).',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `rotate master key failed: ${res.status}`),
    )
  }
  return (await res.json()) as RotateMasterKeyResult
}

// No query invalidation on success: rotation doesn't change anything
// systemStatus.ts's own query reports (secrets_configured stays true,
// it's the same master key slot, just re-wrapped under a new value).
// The caller's own dialog is the only consumer of the result.
export function useRotateMasterKey() {
  return useMutation({
    mutationFn: rotateMasterKey,
  })
}
