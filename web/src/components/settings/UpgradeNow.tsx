import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Button } from '../ui/button'
import { toast } from '../ui/toast'
import {
  preflightQueryOptions,
  updatesQueryOptions,
} from '../../queries/updates'
import {
  selfUpgradeAttemptsQueryOptions,
  selfUpgradePlanQueryOptions,
  type SelfUpgradeAttempt,
  useStartSelfUpgrade,
} from '../../queries/selfUpgrade'
import { scheduleQueryOptions } from '../../queries/upgradeSchedule'
import { CopyCommand } from './CopyCommand'
import { ScheduleUpgradeDialog } from './ScheduleUpgradeDialog'
import { ScheduledUpgradeBanner } from './ScheduledUpgradeBanner'
import { UpgradeCheckRow } from './UpgradeCheckRow'
import { UpgradeConfirmDialog } from './UpgradeConfirmDialog'
import { AttemptRow } from './UpgradeAttempts'

const POLL_MS = 3_000

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
export function UpgradeNow() {
  const { t } = useTranslation('updates')
  const preflight = useQuery(preflightQueryOptions())
  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  const schedule = useQuery(scheduleQueryOptions())
  const start = useStartSelfUpgrade()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [scheduleOpen, setScheduleOpen] = useState(false)
  const [startedAt, setStartedAt] = useState<number | null>(null)

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
  const active =
    running !== undefined || (startedAt !== null && !finishedAttempt)
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
    window.location.reload()
  }, [finishedVersion, target, t])

  if (preflight.isPending || plan.isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('selfUpgrade.loading')}
      </p>
    )
  }
  const pre = preflight.data
  if (!pre || !pre.update_available) return null

  if (active) {
    return (
      <div className="space-y-3" role="status">
        <p className="text-sm font-medium">
          {t('upgradeNow.progressTitle', { version: target })}
        </p>
        {running ? (
          <ul>
            <AttemptRow attempt={running} />
          </ul>
        ) : null}
        <p className="text-xs text-muted-foreground">
          {t('upgradeNow.reconnect')}
        </p>
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
      {finishedAttempt ? (
        <ul>
          <AttemptRow attempt={finishedAttempt} />
        </ul>
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
              <UpgradeCheckRow
                key={c.code}
                check={c}
                cosignCommand={pre.cosign_command}
              />
            ))}
          </ul>
        </div>
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
          <CopyCommand
            label={t('selfUpgrade.hostCommand')}
            command={planData.command}
          />
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          disabled={pre.blocked || !canApply || start.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          {t('upgradeNow.button', { version: target })}
        </Button>
        <Button
          type="button"
          variant="outline"
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
