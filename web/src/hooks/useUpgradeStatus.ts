import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  selfUpgradeAttemptsQueryOptions,
  selfUpgradePlanQueryOptions,
  UPGRADE_POLL_MS,
  type SelfUpgradeAttempt,
} from '../queries/selfUpgrade'
import { buildPipeline, type PipelineView } from '../lib/upgradePhases'
import {
  clearUpgrade,
  rememberUpgrade,
  useUpgradeTrack,
} from '../lib/upgradeTracker'
import { useIsRoot } from './useIsRoot'

export type UpgradePhaseState = 'idle' | 'running' | 'reconnecting' | 'finished'

export interface UpgradeStatus {
  state: UpgradePhaseState
  target: string
  attempt: SelfUpgradeAttempt | undefined
  view: PipelineView
  startedAt: number | null
  lastStep: string
}

const IDLE_POLL_MS = UPGRADE_POLL_MS * 10

// One shared subscription (same cache key as the Updates page) for the global
// bar, the sidebar item and the page. Cached data survives request failures,
// so a restart shows "reconnecting" instead of an error.
export function useUpgradeStatus(): UpgradeStatus {
  const isRoot = useIsRoot()
  const track = useUpgradeTrack()
  const plan = useQuery({
    ...selfUpgradePlanQueryOptions(''),
    enabled: isRoot && track !== null,
  })
  const attempts = useQuery({
    ...selfUpgradeAttemptsQueryOptions(false),
    enabled: isRoot,
    refetchInterval: (q) =>
      (q.state.data ?? []).some((a) => a.outcome === 'running') || track
        ? UPGRADE_POLL_MS
        : IDLE_POLL_MS,
  })
  const list = attempts.data ?? []
  const running = list.find((a) => a.outcome === 'running')
  const tracked = track ? list.find((a) => a.id === track.id) : undefined
  const finished =
    tracked && tracked.outcome !== 'running' ? tracked : undefined
  const attempt = running ?? finished
  const failing = attempts.isError || attempts.failureCount > 0

  useEffect(() => {
    if (running) {
      rememberUpgrade({
        id: running.id,
        target: running.to_version,
        step: running.current_step,
        startedAt: Date.parse(running.started_at),
      })
    } else if (track && !attempts.isPending && !failing && !tracked) {
      clearUpgrade()
    }
  }, [running, track, tracked, attempts.isPending, failing])

  const view = buildPipeline(plan.data?.steps ?? [], attempt)
  let state: UpgradePhaseState = 'idle'
  if (running) state = failing ? 'reconnecting' : 'running'
  else if (finished) state = 'finished'
  else if (track && (failing || attempts.isPending)) state = 'reconnecting'
  return {
    state: isRoot ? state : 'idle',
    target: attempt?.to_version ?? track?.target ?? '',
    attempt,
    view,
    lastStep: view.currentStep || track?.step || '',
    startedAt: attempt
      ? Date.parse(attempt.started_at)
      : (track?.startedAt ?? null),
  }
}
