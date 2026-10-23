import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { isValidTimeZone, nextRuns, parseCron } from '../lib/cronNext'

const PREVIEW_COUNT = 5

// CronPreview lists the next runs of a schedule in its own time zone, so a
// wrong cron or zone shows before the job is saved.
export function CronPreview({
  cron,
  timezone,
  now,
}: {
  cron: string
  timezone: string
  now?: number
}) {
  const { t } = useTranslation('jobs')
  const [mountedAt] = useState(() => Date.now())
  const runs = useMemo(() => {
    const spec = parseCron(cron)
    const tz = timezone.trim() || 'UTC'
    if (!spec || !isValidTimeZone(tz)) return null
    return nextRuns(spec, tz, now ?? mountedAt, PREVIEW_COUNT)
  }, [cron, timezone, now, mountedAt])

  if (runs === null) {
    return (
      <p className="text-xs text-muted-foreground">{t('preview.invalid')}</p>
    )
  }
  if (runs.length === 0) {
    return <p className="text-xs text-muted-foreground">{t('preview.none')}</p>
  }
  const tz = timezone.trim() || 'UTC'
  const fmt = new Intl.DateTimeFormat(undefined, {
    timeZone: tz,
    dateStyle: 'medium',
    timeStyle: 'short',
  })
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-foreground">
        {t('preview.title', { zone: tz })}
      </p>
      <ol className="space-y-0.5 text-xs text-muted-foreground">
        {runs.map((ms) => (
          <li key={ms}>{fmt.format(new Date(ms))}</li>
        ))}
      </ol>
    </div>
  )
}
