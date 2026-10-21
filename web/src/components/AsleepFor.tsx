import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { formatSleepDuration } from '../lib/sleepTimeline'
import { setVisibleInterval } from '../lib/pageVisibility'

const TICK_MS = 30_000

/** AsleepFor renders "Asleep 2h 14m", refreshing as time passes. */
export function AsleepFor({ since }: { since: string }) {
  const { t } = useTranslation('sleepHistory')
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const stop = setVisibleInterval(() => setNow(Date.now()), TICK_MS)
    return () => stop()
  }, [])
  return (
    <>
      {t('badge.asleepFor', {
        duration: formatSleepDuration((now - Date.parse(since)) / 1000),
      })}
    </>
  )
}
