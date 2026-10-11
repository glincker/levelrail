import { useTranslation } from 'react-i18next'
import { StatusPill, type Tone } from '@/components/kit'
import { Button } from '../ui/button'
import { formatInZone } from '../../lib/formatInZone'
import {
  useCancelSchedule,
  type ScheduleState,
  type UpgradeSchedule,
} from '../../queries/upgradeSchedule'

const STATE_TONE: Record<ScheduleState, Tone> = {
  pending: 'info',
  blocked: 'warning',
  running: 'info',
  applied: 'success',
  failed: 'danger',
  cancelled: 'neutral',
  missed: 'warning',
}

export function ScheduledUpgradeBanner({
  schedule,
  serverTimezone,
  onReschedule,
}: {
  schedule: UpgradeSchedule
  serverTimezone: string
  onReschedule?: () => void
}) {
  const { t } = useTranslation('updates')
  const cancel = useCancelSchedule()
  const cancellable =
    schedule.state === 'pending' || schedule.state === 'blocked'
  return (
    <div
      className="space-y-2 rounded-md border border-border px-3 py-3"
      role="status"
    >
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <StatusPill
          tone={STATE_TONE[schedule.state]}
          label={t(`schedule.state.${schedule.state}`)}
          size="sm"
        />
        <span className="font-medium">
          {t('schedule.bannerTitle', {
            version: schedule.target_version,
            time: formatInZone(schedule.run_at, serverTimezone),
            timezone: serverTimezone,
          })}
        </span>
        {schedule.source === 'auto' ? (
          <span className="text-xs text-muted-foreground">
            {t('schedule.automatic')}
          </span>
        ) : null}
      </div>
      {schedule.state === 'blocked' && schedule.last_reason ? (
        <p className="text-xs text-foreground">
          {t('schedule.blockedReason', {
            reason: schedule.last_reason,
            until: formatInZone(schedule.retry_until, serverTimezone),
          })}
        </p>
      ) : null}
      {cancellable ? (
        <div className="flex flex-wrap gap-2">
          {onReschedule ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onReschedule}
            >
              {t('schedule.reschedule')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={cancel.isPending}
            onClick={() => cancel.mutate()}
          >
            {t('schedule.cancelSchedule')}
          </Button>
        </div>
      ) : null}
      {cancel.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {cancel.error.message}
        </p>
      ) : null}
    </div>
  )
}
