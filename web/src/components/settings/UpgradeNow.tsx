import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { toast } from '../ui/toast'
import {
  preflightQueryOptions,
  updatesQueryOptions,
} from '../../queries/updates'
import {
  selfUpgradeAttemptsQueryOptions,
  UPGRADE_POLL_MS,
  selfUpgradePlanQueryOptions,
  selfUpgradeStatusQueryOptions,
  type SelfUpgradeAttempt,
  UpgradeRunningError,
  useStartSelfUpgrade,
} from '../../queries/selfUpgrade'
import { isSeen, markSeen } from '../../lib/upgradeSeen'
import { scheduleQueryOptions } from '../../queries/upgradeSchedule'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { CliFallback } from './CliFallback'
import { CopyCommand } from './CopyCommand'
import { ScheduleUpgradeDialog } from './ScheduleUpgradeDialog'
import { ScheduledUpgradeBanner } from './ScheduledUpgradeBanner'
import { UpgradeCheckRow } from './UpgradeCheckRow'
import { UpgradeConfirmDialog } from './UpgradeConfirmDialog'
import { EdgeUpdateGuidance } from './EdgeUpdateGuidance'
import { UpgradeProgress } from './UpgradeProgress'
import { UpgradeStateHero } from './UpgradeStateHero'
import { UpgradePlanPreview } from './UpgradePlanPreview'

const POLL_MS = UPGRADE_POLL_MS
const RECENT_MS = 15 * 60_000
const RELOAD_DELAY_MS = 5_000

function isRecentFinished(a: SelfUpgradeAttempt): boolean {
  if (a.outcome === 'running') return false
  const at = Date.parse(a.finished_at || a.started_at)
  return Date.now() - at < RECENT_MS
}

function findFinished(
  list: SelfUpgradeAttempt[],
  target: string,
  startedAt: number,
): SelfUpgradeAttempt | undefined {
  return list.find(
    (a) =>
      a.to_version === target &&
      a.outcome !== 'running' &&
      Date.parse(a.started_at) >= startedAt - 60_000,
  )
}

// UpgradeNow is the primary action on the Version card: blockers with their
// fixes while the preflight fails, then Upgrade now and Schedule upgrade,
// then live progress that reloads the page once the new release answers.
// An edge target is a container image, so it shows pull guidance instead.
export function UpgradeNow() {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  const schedule = useQuery(scheduleQueryOptions())
  const start = useStartSelfUpgrade()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [scheduleOpen, setScheduleOpen] = useState(false)
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [dismissed, setDismissed] = useState('')
  const [rechecking, setRechecking] = useState(false)
  const queryClient = useQueryClient()

  const attempts = useQuery({
    ...selfUpgradeAttemptsQueryOptions(false),
    refetchInterval: (q) => {
      const list = q.state.data ?? []
      if (list.some((a) => a.outcome === 'running')) return POLL_MS
      if (startedAt === null) return false
      return findFinished(list, target, startedAt) ? false : POLL_MS
    },
  })
  const target = plan.data?.target_version ?? ''
  const running = (attempts.data ?? []).find((a) => a.outcome === 'running')
  const finishedAttempt =
    startedAt === null
      ? undefined
      : findFinished(attempts.data ?? [], target, startedAt)
  const serverStatus = useQuery(selfUpgradeStatusQueryOptions())
  const active =
    running !== undefined ||
    Boolean(serverStatus.data?.running) ||
    (startedAt !== null && !finishedAttempt)
  const status = useQuery({
    ...updatesQueryOptions(),
    refetchInterval: active ? POLL_MS : false,
  })

  const finishedVersion =
    active && target !== '' && status.data?.current_version === target
  useEffect(() => {
    if (!finishedVersion) return
    toast.add({
      title: t('upgradeNow.done', { version: target }),
      type: 'success',
    })
    const id = window.setTimeout(
      () => window.location.reload(),
      RELOAD_DELAY_MS,
    )
    return () => window.clearTimeout(id)
  }, [finishedVersion, target, t])
  const unseenFinished =
    finishedAttempt && !isSeen('panel', finishedAttempt.id)
      ? finishedAttempt
      : undefined
  const sessionSucceededId =
    unseenFinished?.outcome === 'succeeded' ? unseenFinished.id : ''
  useEffect(() => {
    if (!sessionSucceededId) return
    const onHide = () => markSeen('panel', sessionSucceededId)
    window.addEventListener('pagehide', onHide)
    return () => window.removeEventListener('pagehide', onHide)
  }, [sessionSucceededId])
  const olderFailure = (attempts.data ?? []).find(
    (a) =>
      a.id !== dismissed &&
      a.outcome !== 'succeeded' &&
      !isSeen('panel', a.id) &&
      isRecentFinished(a),
  )
  const recent = unseenFinished ?? olderFailure
  const planSteps = plan.data?.steps ?? []
  const reconnecting = attempts.isError || status.isError

  if (preflight.isPending || plan.isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('selfUpgrade.loading')}
      </p>
    )
  }
  const pre = preflight.data
  const hostBinaryEdge = pre?.edge?.installable === false
  if (!pre || (!pre.update_available && !hostBinaryEdge)) {
    const schemaBlock = pre?.checks.find(
      (c) => c.code === 'schema_compat' && c.status === 'fail',
    )
    if (!recent && pre && (pre.state === 'ahead_of_releases' || schemaBlock)) {
      return (
        <UpgradeStateHero
          pre={pre}
          checking={rechecking}
          onCheckAgain={() => {
            setRechecking(true)
            void fetch('/api/v1/updates/preflight?refresh=1')
              .then(() =>
                queryClient.invalidateQueries({ queryKey: ['updates'] }),
              )
              .finally(() => setRechecking(false))
          }}
        />
      )
    }
    if (!recent) return null
    return (
      <UpgradeProgress
        target={recent.to_version}
        plannedSteps={planSteps}
        attempt={recent}
        startedAt={null}
        reconnecting={false}
        releaseUrl={pre?.release_url}
        onDismiss={() => {
          markSeen('panel', recent.id)
          setDismissed(recent.id)
        }}
      />
    )
  }

  if (active) {
    return (
      <UpgradeProgress
        target={running?.to_version ?? target}
        plannedSteps={planSteps}
        attempt={running}
        startedAt={startedAt}
        reconnecting={reconnecting}
        releaseUrl={pre.release_url}
      />
    )
  }

  if (pre.edge) {
    return (
      <div className="space-y-4">
        {schedule.data?.active ? (
          <ScheduledUpgradeBanner
            schedule={schedule.data.active}
            serverTimezone={schedule.data.server_timezone ?? 'UTC'}
          />
        ) : null}
        <EdgeUpdateGuidance
          edge={pre.edge}
          updateAvailable={pre.update_available}
        />
      </div>
    )
  }

  const blockers = pre.checks.filter(
    (c) => c.status === 'fail' || c.status === 'pending',
  )
  const planData = plan.data
  const serverTimezone = schedule.data?.server_timezone ?? 'UTC'
  const activeSchedule = schedule.data?.active ?? null
  const canApply = planData?.can_apply === true && planData.notes_available

  return (
    <div className="space-y-4">
      {recent ? (
        <UpgradeProgress
          target={recent.to_version}
          plannedSteps={planSteps}
          attempt={recent}
          startedAt={null}
          reconnecting={false}
          releaseUrl={pre.release_url}
          onDismiss={() => {
            markSeen('panel', recent.id)
            setDismissed(recent.id)
            setStartedAt(null)
          }}
          onRetry={() => {
            markSeen('panel', recent.id)
            setDismissed(recent.id)
            setStartedAt(null)
            setConfirmOpen(true)
          }}
        />
      ) : null}
      {activeSchedule ? (
        <ScheduledUpgradeBanner
          schedule={activeSchedule}
          serverTimezone={serverTimezone}
          onReschedule={() => setScheduleOpen(true)}
        />
      ) : null}
      {blockers.length > 0 ? (
        <div className="space-y-2">
          <p className="text-sm font-medium">
            {pre.waiting
              ? t('upgradeNow.waitingTitle')
              : t('upgradeNow.blockedTitle', { count: blockers.length })}
          </p>
          <ul className="space-y-3" aria-label={t('preflight.checksLabel')}>
            {blockers.map((c) => (
              <UpgradeCheckRow key={c.code} check={c} />
            ))}
          </ul>
        </div>
      ) : null}
      <UpgradePlanPreview pre={pre} />
      {planData && planData.signature_pinned === false ? (
        <p className="flex items-start gap-2 text-sm text-muted-foreground">
          <ShieldCheckGlassIcon
            aria-hidden
            className="mt-0.5 size-4.5 shrink-0 text-tone-success"
          />
          {t('selfUpgrade.notPinned')}
        </p>
      ) : null}
      {planData?.signature_pinned ? (
        <p className="text-xs text-muted-foreground">
          {t('selfUpgrade.pinned', {
            version: planData.signature_pinned_version ?? '',
          })}
        </p>
      ) : null}
      {planData && !planData.notes_available ? (
        <p className="text-xs text-muted-foreground">
          {t('selfUpgrade.notesMissing')}
        </p>
      ) : null}
      {planData && !planData.can_apply ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t('selfUpgrade.hostOnly', {
              reason: planData.cannot_apply_reason ?? '',
            })}
          </p>
          <CliFallback>
            <CopyCommand
              label={t('selfUpgrade.hostCommand')}
              command={planData.command}
            />
          </CliFallback>
        </div>
      ) : null}
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Button
          type="button"
          className="w-full sm:w-auto"
          disabled={pre.blocked || !canApply || start.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          {t('upgradeNow.button', { version: target })}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="w-full sm:w-auto"
          disabled={!canApply || schedule.data?.available === false}
          onClick={() => setScheduleOpen(true)}
        >
          {t('schedule.button')}
        </Button>
        {pre.blocked && !pre.waiting ? (
          <span className="text-xs text-muted-foreground">
            {t('upgradeNow.fixFirst')}
          </span>
        ) : null}
      </div>
      {planData ? (
        <>
          <UpgradeConfirmDialog
            open={confirmOpen}
            onOpenChange={setConfirmOpen}
            plan={planData}
            pending={start.isPending}
            error={start.isError ? start.error.message : undefined}
            onConfirm={(ack) =>
              start.mutate(
                { target: planData.target_version, ack },
                {
                  onSuccess: () => {
                    setConfirmOpen(false)
                    setStartedAt(Date.now())
                  },
                  onError: (err) => {
                    if (!(err instanceof UpgradeRunningError)) return
                    setConfirmOpen(false)
                    toast.add({
                      title: t('result.running', {
                        to: target,
                        id: err.attemptId,
                      }),
                      type: 'info',
                    })
                  },
                },
              )
            }
          />
          <ScheduleUpgradeDialog
            open={scheduleOpen}
            onOpenChange={setScheduleOpen}
            target={planData.target_version}
            breaking={planData.breaking}
            serverTimezone={serverTimezone}
            nextWindow={schedule.data?.next_window ?? null}
            replacing={activeSchedule !== null}
          />
        </>
      ) : null}
    </div>
  )
}
