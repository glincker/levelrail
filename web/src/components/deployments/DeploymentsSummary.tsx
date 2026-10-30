import { useTranslation } from 'react-i18next'
import {
  ActivityGlassIcon,
  ChartColumnGlassIcon,
  CircleCheckGlassIcon,
  ClockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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

export function DeploymentsSummary({
  summary,
  loading,
  failed,
  onNeedsAttention,
}: DeploymentsSummaryProps) {
  const { t } = useTranslation('deploys')
  const na = t('summary.na')
  const naTip = t('summary.unavailableTip')
  const unavailable = failed && !summary
  const s = summary
  const median = s?.duration.median_ms ?? null
  const attention = s?.needs_attention ?? 0
  const series = s ? s.per_day.map((d) => d.total) : []
  const total14 = series.reduce((a, b) => a + b, 0)
  const queued = s?.counts.queued ?? 0

  return (
    <section
      aria-label={t('summary.label')}
      className="grid grid-cols-2 gap-3 lg:grid-cols-5"
    >
      <MetricTile
        label={t('summary.inProgress')}
        loading={loading}
        icon={<ActivityGlassIcon size="1.4em" />}
        value={unavailable ? na : (s?.in_progress ?? 0)}
        tone={s && s.in_progress > 0 ? 'info' : 'neutral'}
        info={unavailable ? naTip : t('summary.inProgressTip', { queued })}
      />
      <MetricTile
        label={t('summary.failureRate')}
        loading={loading}
        icon={
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
        }
        value={
          unavailable ? na : formatFailureRate(s?.failure_rate_24h ?? null)
        }
        info={unavailable ? naTip : t('summary.failureRateTip')}
      />
      <MetricTile
        label={t('summary.medianDuration')}
        loading={loading}
        icon={<ClockGlassIcon size="1.4em" />}
        value={
          unavailable || median === null ? na : (formatDurationMs(median) ?? na)
        }
        info={unavailable ? naTip : t('summary.medianDurationTip')}
      />
      <MetricTile
        label={t('summary.needsAttention')}
        loading={loading}
        icon={
          <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
        }
        value={unavailable ? na : attention}
        tone={attention > 0 ? 'warning' : 'neutral'}
        onClick={attention > 0 ? onNeedsAttention : undefined}
        info={unavailable ? naTip : t('summary.needsAttentionTip')}
      />
      <div className="col-span-2 lg:col-span-1">
        <MetricTile
          label={t('summary.deploys14')}
          loading={loading}
          icon={<ChartColumnGlassIcon size="1.4em" />}
          value={unavailable ? na : total14}
          series={unavailable ? undefined : series}
          info={unavailable ? naTip : t('summary.deploys14Tip')}
        />
      </div>
    </section>
  )
}
