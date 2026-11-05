// Cancel and retry for a running self upgrade. The paths live here so a
// backend rename is a one line change.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { selfUpgradeKeys } from './selfUpgrade'

const attemptUrl = (id: string, action: string) =>
  `/api/v1/updates/self-upgrade/attempts/${encodeURIComponent(id)}/${action}`

export const upgradeCancelUrl = (id: string) => attemptUrl(id, 'cancel')
export const upgradeRetryUrl = (id: string) => attemptUrl(id, 'retry')

function useUpgradeControl(url: (id: string) => string, what: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (attemptId: string): Promise<void> => {
      const res = await fetch(url(attemptId), { method: 'POST' })
      if (!res.ok) {
        // A 409 carries a user-facing reason (applying or already finished).
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `${what} failed: ${res.status}`),
        )
      }
    },
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.all }),
  })
}

export function useCancelUpgrade() {
  return useUpgradeControl(upgradeCancelUrl, 'cancel upgrade')
}

export function useRetryUpgrade() {
  return useUpgradeControl(upgradeRetryUrl, 'retry upgrade')
}
