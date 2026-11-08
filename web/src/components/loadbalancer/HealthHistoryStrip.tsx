import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { CheckSample } from '../../queries/loadBalancerLive'
import { BOXES, summarizeChecks } from './checkSummary'

function boxTitle(c: CheckSample, t: TFunction<'statePlaceholders'>): string {
  const when = new Date(c.at).toLocaleTimeString()
  if (c.ok)
    return c.latency_ms
      ? t('lb.history.passedIn', { when, ms: c.latency_ms })
      : t('lb.history.passed', { when })
  if (c.reason) return t('lb.history.failedReason', { when, reason: c.reason })
  if (c.status_code)
    return t('lb.history.failedStatus', { when, status: c.status_code })
  return t('lb.history.failed', { when })
}

// Filled = passed, hatched = failed, hollow = no check yet. Shape carries the
// meaning so it survives without color.
export function HealthHistoryStrip({
  checks,
  className,
}: {
  checks: CheckSample[]
  className?: string
}) {
  const { t } = useTranslation('statePlaceholders')
  const last = checks.slice(-BOXES)
  const empty = BOXES - last.length
  return (
    <span
      role="img"
      aria-label={last.length ? summarizeChecks(checks) : t('lb.history.none')}
      className={cn('inline-flex items-center gap-[3px]', className)}
    >
      {Array.from({ length: empty }, (_, i) => (
        <span
          key={`empty-${i}`}
          aria-hidden="true"
          className="size-3 rounded-[3px] border border-dashed border-border"
        />
      ))}
      {last.map((c, i) => (
        <span
          key={`${c.at}-${i}`}
          aria-hidden="true"
          title={boxTitle(c, t)}
          data-check={c.ok ? 'pass' : 'fail'}
          className={cn(
            'size-3 rounded-[3px] border',
            c.ok
              ? 'border-tone-success-solid bg-tone-success-solid'
              : 'border-tone-danger-solid bg-[repeating-linear-gradient(135deg,var(--tone-danger-solid)_0_2px,transparent_2px_4px)]',
          )}
        />
      ))}
    </span>
  )
}
