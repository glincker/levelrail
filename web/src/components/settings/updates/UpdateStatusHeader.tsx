import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import {
  preflightQueryOptions,
  updatesQueryOptions,
  type UpdateStatus,
} from '@/queries/updates'
import {
  selfUpgradeAttemptsQueryOptions,
  selfUpgradeStatusQueryOptions,
  type SelfUpgradeAttempt,
} from '@/queries/selfUpgrade'
import { ATTEMPTS_SECTION_ID } from '@/lib/updatesAnchors'
import { deriveActions, isStalled, useLastChange } from '@/lib/upgradeControl'
import { useNow } from '@/lib/upgradeTime'
import { useCancelUpgrade, useRetryUpgrade } from '@/queries/upgradeControl'
import { useRestartControlPlane } from '@/queries/systemRestart'
import {
  selfUpgradePlanQueryOptions,
  UpgradeRunningError,
  useStartSelfUpgrade,
} from '@/queries/selfUpgrade'
import { UpgradeConfirmDialog } from '../UpgradeConfirmDialog'
import { UpgradeFailureNote } from './UpgradeFailureNote'
import { RestartServerDialog } from './RestartServerDialog'
import { UpdateActionBar } from './UpdateActionBar'
import { UpgradeLiveProgress } from './UpgradeLiveProgress'
import { markSeen, useSeen } from '@/lib/upgradeSeen'
import { UpgradeResultCard } from './UpgradeResultCard'
import { scheduleQueryOptions } from '@/queries/upgradeSchedule'
import { formatRelative } from '@/components/kit/formatRelative'
import { UpgradeNow } from '../UpgradeNow'
import { UpdateChanges } from './UpdateChanges'
import { deriveUpdateState } from './updateState'
import { UpdateStatusView } from './UpdateStatusView'
import { UpgradeStepper } from './UpgradeStepper'
import { deriveStepper } from '@/lib/upgradeStepper'

const RECENT_MS = 15 * 60_000
const RELATIVE_TICK_MS = 30_000

function recentFinished(
  list: SelfUpgradeAttempt[],
): SelfUpgradeAttempt | undefined {
  const last = list
    .filter((a) => a.outcome !== 'running')
    .sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))[0]
  return last && Date.now() - Date.parse(last.started_at) < RECENT_MS
    ? last
    : undefined
}

// UpdateStatusHeader owns the page's top card: derived state, the manual
// re-check, the change preview, and the existing upgrade controls.
export function UpdateStatusHeader({ initial }: { initial: UpdateStatus }) {
  const { t } = useTranslation('updates')
  const [checking, setChecking] = useState(false)
  const query = useQuery({ ...updatesQueryOptions(), initialData: initial })
  const preflight = useQuery(preflightQueryOptions())
  const schedule = useQuery(scheduleQueryOptions())
  const attempts = useQuery(selfUpgradeAttemptsQueryOptions(false))
  const upgradeStatus = useQuery(selfUpgradeStatusQueryOptions())
  const [manualCheckAt, setManualCheckAt] = useState<number | null>(null)
  const [, setTick] = useState(0)
  useEffect(() => {
    const id = window.setInterval(() => setTick((n) => n + 1), RELATIVE_TICK_MS)
    return () => window.clearInterval(id)
  }, [])

  const status = query.data
  const edge = status.release_kind === 'edge_image'
  const state = deriveUpdateState({
    latestVersion: status.latest_version,
    updateAvailable: status.update_available,
    hostBinaryEdge: edge && preflight.data?.edge?.installable === false,
    scheduleActive:
      schedule.data?.active?.state === 'pending' ||
      schedule.data?.active?.state === 'blocked' ||
      schedule.data?.active?.state === 'running',
    upgradeRunning: (attempts.data ?? []).some((a) => a.outcome === 'running'),
    checkFailed: query.isRefetchError,
  })

  const list = attempts.data ?? []
  const running = list.find((a) => a.outcome === 'running')
  const recentDone = recentFinished(list)
  const last = upgradeStatus.data?.last_finished ?? null
  const panelSeen = useSeen('panel', recentDone?.id ?? '')
  const cardDismissed = useSeen('card', last?.id ?? '')
  const lastPanelSeen = useSeen('panel', last?.id ?? '')
  const showCard =
    last !== null &&
    last.outcome !== 'refused' &&
    !cardDismissed &&
    (last.outcome === 'succeeded' ||
      lastPanelSeen ||
      recentFinished([last]) === undefined)
  const hideStepper = running !== undefined || (recentDone && !panelSeen)
  const steps = deriveStepper({
    updateAvailable: status.update_available,
    checkFailed: query.isRefetchError,
    preflightBlocked: preflight.data?.blocked === true,
    attempt: running,
  })

  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  const start = useStartSelfUpgrade()
  const cancel = useCancelUpgrade()
  const retry = useRetryUpgrade()
  const restart = useRestartControlPlane({ inlineError: true })
  const [updateOpen, setUpdateOpen] = useState(false)
  const [restartOpen, setRestartOpen] = useState(false)
  const live = upgradeStatus.data?.progress ?? null
  const runningAttempt = upgradeStatus.data?.running ?? running ?? null
  const isRunning = runningAttempt !== null
  const now = useNow(isRunning)
  const lastChangeAt = useLastChange(
    [
      live?.bytes_downloaded ?? '',
      runningAttempt?.current_step ?? '',
      runningAttempt?.steps.length ?? 0,
      runningAttempt?.log.length ?? 0,
    ].join('|'),
    isRunning,
  )
  const stalled =
    isRunning &&
    isStalled({
      serverStalled: live?.stalled,
      lastProgressAt: live?.last_progress_at,
      lastChangeAt,
      now,
    })
  const lastFinished = upgradeStatus.data?.last_finished ?? recentDone ?? null
  const lastOutcome = lastFinished?.outcome
  const failedNow =
    !isRunning && (lastOutcome === 'failed' || lastOutcome === 'rolled_back')
  const actions = deriveActions({
    running: isRunning,
    stalled,
    failed: failedNow && !showCard,
    updateAvailable: status.update_available && !edge,
    canApply: plan.data?.can_apply === true && !preflight.data?.blocked,
    step: runningAttempt?.current_step ?? '',
    canCancel: live?.can_cancel,
    canRetry: live?.can_retry,
    cancelRequested: live?.cancel_requested,
  })
  const busy = cancel.isPending || retry.isPending || restart.isPending
  const startedAtMs = Date.parse(
    live?.started_at ?? runningAttempt?.started_at ?? '',
  )
  const lastProgressRaw = live?.last_progress_at
    ? Date.parse(live.last_progress_at)
    : 0
  const failing = upgradeStatus.isError || upgradeStatus.failureCount > 0

  const fail = (e: Error) => toast.add({ title: e.message, type: 'error' })
  const attemptId = live?.attempt_id ?? runningAttempt?.id ?? ''
  const retryAfterCancel = useRef(false)
  const cancelRun = () => cancel.mutate(attemptId, { onError: fail })
  // The server retries only a failed attempt, so a stuck run is cancelled
  // first and retried once it has failed.
  const retryFailedId = failedNow ? (lastFinished?.id ?? '') : ''
  useEffect(() => {
    if (!retryAfterCancel.current || retryFailedId === '') return
    retryAfterCancel.current = false
    retry.mutate(retryFailedId, { onError: fail })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [retryFailedId])
  const onRetry = () => {
    if (isRunning) {
      retryAfterCancel.current = true
      cancelRun()
    } else if (retryFailedId !== '' && live?.can_retry !== false) {
      retry.mutate(retryFailedId, { onError: fail })
    } else setUpdateOpen(true)
  }
  const [skipBackup, setSkipBackup] = useState(false)

  const check = () => {
    setChecking(true)
    setManualCheckAt(Date.now())
    void query.refetch().then((r) => {
      setChecking(false)
      toast.add(
        r.isError
          ? { title: t('status.checkFailedTitle'), type: 'error' }
          : { title: t('status.checked'), type: 'info' },
      )
    })
  }

  const serverCheckedAt = status.checked_at ? Date.parse(status.checked_at) : 0
  const checkedAtMs = Math.max(serverCheckedAt, manualCheckAt ?? 0)
  const showChanges = status.update_available && !edge
  return (
    <UpdateStatusView
      state={state}
      currentVersion={status.current_version}
      latestVersion={status.latest_version}
      edge={edge}
      lastChecked={
        checkedAtMs > 0 ? formatRelative(new Date(checkedAtMs)) : null
      }
      checking={checking}
      errorMessage={query.error?.message}
      releaseUrl={status.release_url}
      onCheck={check}
      hideActions
    >
      <UpdateActionBar
        actions={actions}
        targetVersion={plan.data?.target_version ?? status.latest_version ?? ''}
        checking={checking}
        busy={busy}
        releaseUrl={status.release_url}
        onCheck={check}
        onUpdate={() => setUpdateOpen(true)}
        onCancel={cancelRun}
        onRetry={onRetry}
        onRestart={() => setRestartOpen(true)}
        onViewLogs={() =>
          document
            .getElementById(ATTEMPTS_SECTION_ID)
            ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
        }
      />
      {isRunning ? (
        <UpgradeLiveProgress
          step={runningAttempt.current_step}
          startedAtMs={Number.isNaN(startedAtMs) ? now : startedAtMs}
          lastProgressMs={Math.max(lastProgressRaw, lastChangeAt)}
          now={now}
          downloadedBytes={live?.bytes_downloaded}
          totalBytes={live?.total_bytes ?? undefined}
          stalled={stalled}
          reconnecting={failing}
          actions={actions}
          busy={busy}
          onCancel={cancelRun}
          onRetry={onRetry}
          onRestart={() => setRestartOpen(true)}
          onRetryNow={() => void upgradeStatus.refetch()}
        />
      ) : null}
      <RestartServerDialog
        open={restartOpen}
        onOpenChange={setRestartOpen}
        pending={restart.isPending}
        error={restart.isError ? restart.error.message : undefined}
        onConfirm={() =>
          restart.mutate(undefined, { onSuccess: () => setRestartOpen(false) })
        }
      />
      {plan.data ? (
        <UpgradeConfirmDialog
          open={updateOpen}
          onOpenChange={setUpdateOpen}
          plan={plan.data}
          pending={start.isPending}
          error={start.isError ? start.error.message : undefined}
          onConfirm={(ack) =>
            start.mutate(
              {
                target: plan.data.target_version,
                ack,
                ...(skipBackup ? { skip_backup: true } : {}),
              },
              {
                onSuccess: () => setUpdateOpen(false),
                onError: (err) => {
                  if (err instanceof UpgradeRunningError) setUpdateOpen(false)
                },
              },
            )
          }
        />
      ) : null}
      {hideStepper ? null : <UpgradeStepper steps={steps} />}
      {showCard && last ? (
        <UpgradeResultCard
          attempt={last}
          verified={upgradeStatus.data?.verified === true}
          releaseUrl={
            status.latest_version === last.to_version
              ? status.release_url
              : null
          }
          onDismiss={() => markSeen('card', last.id)}
        />
      ) : null}
      {showChanges ? <UpdateChanges status={status} /> : null}
      {failedNow && !showCard && lastFinished?.failure ? (
        <UpgradeFailureNote failure={lastFinished.failure} />
      ) : null}
      <UpgradeNow
        externalActions
        skipBackup={skipBackup}
        onSkipBackupChange={setSkipBackup}
        onRequestUpgrade={() => setUpdateOpen(true)}
        stalled={stalled}
      />
    </UpdateStatusView>
  )
}
