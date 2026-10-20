import {
  CheckCircleIcon,
  PulseIcon,
  TimerIcon,
  WarningIcon,
  ChartBarIcon,
} from '@phosphor-icons/react/dist/ssr'
import { MetricTile } from '@/components/kit'
import { formatDurationMs } from '../../lib/deployDuration'
import { formatFailureRate } from '../../lib/deploymentPresentation'
import type { DeploymentsSummary as Summary } from '../../types/deployment'

export interface DeploymentsSummaryProps {
  summary: Summary | undefined
  loading: boolean
  failed: boolean
  onNeedsAttention: () => void
}

const NA_TIP = 'The summary could not be loaded. The list below still works.'

export function DeploymentsSummary({
  summary,
  loading,
  failed,
  onNeedsAttention,
}: DeploymentsSummaryProps) {
  const unavailable = failed && !summary
  const s = summary
  const median = s?.duration.median_ms ?? null
  const attention = s?.needs_attention ?? 0
  const series = s ? s.per_day.map((d) => d.total) : []
  const total14 = series.reduce((a, b) => a + b, 0)
  const queued = s?.counts.queued ?? 0

  return (
    <section
      aria-label="Deployment summary"
      className="grid grid-cols-2 gap-3 lg:grid-cols-5"
    >
      <MetricTile
        label="In progress"
        loading={loading}
        icon={<PulseIcon />}
        value={unavailable ? 'n/a' : (s?.in_progress ?? 0)}
        tone={s && s.in_progress > 0 ? 'info' : 'neutral'}
        info={
          unavailable
            ? NA_TIP
            : `Deploys building right now. ${String(queued)} queued.`
        }
      />
      <MetricTile
        label="Failure rate (24h)"
        loading={loading}
        icon={<WarningIcon />}
        value={
          unavailable ? 'n/a' : formatFailureRate(s?.failure_rate_24h ?? null)
        }
        info={
          unavailable
            ? NA_TIP
            : 'Failed deploys as a share of deploys that finished in the last 24 hours.'
        }
      />
      <MetricTile
        label="Median duration"
        loading={loading}
        icon={<TimerIcon />}
        value={
          unavailable || median === null
            ? 'n/a'
            : (formatDurationMs(median) ?? 'n/a')
        }
        info={
          unavailable
            ? NA_TIP
            : 'Median time of deploys that finished in the last 24 hours.'
        }
      />
      <MetricTile
        label="Needs attention"
        loading={loading}
        icon={<CheckCircleIcon />}
        value={unavailable ? 'n/a' : attention}
        tone={attention > 0 ? 'warning' : 'neutral'}
        onClick={attention > 0 ? onNeedsAttention : undefined}
        info={
          unavailable
            ? NA_TIP
            : 'Held deploys, and live images that differ from what is running. Select to show held deploys.'
        }
      />
      <div className="col-span-2 lg:col-span-1">
        <MetricTile
          label="Deploys (14 days)"
          loading={loading}
          icon={<ChartBarIcon />}
          value={unavailable ? 'n/a' : total14}
          series={unavailable ? undefined : series}
          info={
            unavailable
              ? NA_TIP
              : 'Deploys started per day over the last 14 days.'
          }
        />
      </div>
    </section>
  )
}
