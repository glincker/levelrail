import { cn } from '@/lib/utils'
import type { CheckSample } from '../../queries/loadBalancerLive'
import { BOXES, summarizeChecks } from './checkSummary'

function boxTitle(c: CheckSample): string {
  const when = new Date(c.at).toLocaleTimeString()
  if (c.ok)
    return `${when}: passed${c.latency_ms ? ` in ${c.latency_ms}ms` : ''}`
  return `${when}: failed${c.reason ? `, ${c.reason}` : c.status_code ? `, status ${c.status_code}` : ''}`
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
  const last = checks.slice(-BOXES)
  const empty = BOXES - last.length
  return (
    <span
      role="img"
      aria-label={
        last.length ? summarizeChecks(checks) : 'No checks recorded yet'
      }
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
          title={boxTitle(c)}
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
