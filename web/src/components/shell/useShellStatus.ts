import { useQuery } from '@tanstack/react-query'
import { updatesQueryOptions } from '../../queries/updates'
import { useUpgradeStatus } from '../../hooks/useUpgradeStatus'
import { useRestartState } from '../../lib/restartTracker'
import { deriveShellStatus, type ShellStatus } from '../../lib/shellStatus'
import { useAttentionSummary } from './useAttentionSummary'

const UPDATE_STALE_MS = 5 * 60_000

// Reuses the header chip's attention cache, the Updates page key and the shared upgrade subscription: no extra polling.
export function useShellStatus(): ShellStatus {
  const summary = useAttentionSummary()
  const updates = useQuery({
    ...updatesQueryOptions(),
    staleTime: UPDATE_STALE_MS,
    retry: false,
  })
  const upgrade = useUpgradeStatus()
  const restart = useRestartState()
  return deriveShellStatus({
    level: summary.isLoading ? null : summary.level,
    updateAvailable: updates.data?.update_available === true,
    upgrading: upgrade.state === 'running' || upgrade.state === 'reconnecting',
    restarting: restart?.phase === 'restarting',
  })
}
