import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RelativeTime } from '../kit/RelativeTime'
import { sleepCountdown } from '../../lib/sleepState'
import type { SleepActivity } from '../../queries/appSleep'
import { setVisibleInterval } from '../../lib/pageVisibility'

const TICK_MS = 1000

function Item({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm font-medium text-foreground">
        {children}
      </dd>
    </div>
  )
}

/** SleepActivityStrip shows what keeps an awake app awake and counts down to its sleep. */
export function SleepActivityStrip({ activity }: { activity: SleepActivity }) {
  const { t } = useTranslation('deploys')
  const willSleepAt = activity.will_sleep_at
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!willSleepAt) return
    const stop = setVisibleInterval(() => setNow(Date.now()), TICK_MS)
    return () => stop()
  }, [willSleepAt])

  const countdown = sleepCountdown(willSleepAt, now)
  const none = t('appSleep.activity.none')
  return (
    <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Item label={t('appSleep.activity.lastRequest')}>
        {activity.last_request_at ? (
          <RelativeTime at={activity.last_request_at} live />
        ) : (
          none
        )}
      </Item>
      <Item label={t('appSleep.activity.lastNetwork')}>
        {activity.last_network_activity_at ? (
          <RelativeTime at={activity.last_network_activity_at} live />
        ) : (
          none
        )}
      </Item>
      <Item label={t('appSleep.activity.connections')}>
        {activity.open_connections}
      </Item>
      <Item label={t('appSleep.activity.sleepsIn')}>
        {countdown ? (
          <span data-testid="sleep-countdown">
            {countdown.unit === 'minutes'
              ? t('appSleep.countdown.minutes', { count: countdown.value })
              : t('appSleep.countdown.seconds', { count: countdown.value })}
          </span>
        ) : (
          t('appSleep.activity.notScheduled')
        )}
      </Item>
    </dl>
  )
}
