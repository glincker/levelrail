import { useTranslation } from 'react-i18next'
import { Sparkline } from '@/components/kit'
import { ApiError } from '@/lib/apiError'
import { ERROR_RATE_MIN_REQUESTS } from '@/lib/fleetThresholds'
import {
  TRAFFIC_WINDOW_SECONDS,
  errorReading,
  summarizeTraffic,
} from '@/lib/fleetStats'
import { useAppsMetrics } from '@/queries/appsMetrics'
import { BigStat, Muted, WidgetSkeleton } from './parts'

/** Requests, error share and p95 across all apps, from the batch metrics read. */
export function FleetTrafficWidget() {
  const { t } = useTranslation('dashboards')
  const q = useAppsMetrics()
  if (q.isLoading) return <WidgetSkeleton />
  if (q.error instanceof ApiError && q.error.status === 501) {
    return <Muted>{t('fleet.traffic.notConfigured')}</Muted>
  }
  if (q.error || !q.data) return <Muted>{t('fleet.loadFailed')}</Muted>
  const sum = summarizeTraffic(q.data)
  if (sum.reporting === 0) return <Muted>{t('fleet.traffic.empty')}</Muted>
  const err = errorReading(sum, ERROR_RATE_MIN_REQUESTS)
  // p95 across apps is weighted by each app's request rate.
  const p95 =
    sum.ratePerSec > 0
      ? q.data.reduce(
          (a, r) => a + (r.has_traffic ? r.p95_ms * r.rate_per_sec : 0),
          0,
        ) / sum.ratePerSec
      : 0
  const requests = Math.round(sum.ratePerSec * TRAFFIC_WINDOW_SECONDS)
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div className="flex flex-wrap gap-x-6 gap-y-2">
        <BigStat
          value={requests.toLocaleString()}
          label={t('fleet.traffic.requests')}
        />
        <BigStat
          value={
            err.kind === 'value'
              ? `${err.pct.toFixed(2)}%`
              : t('fleet.traffic.tooFew')
          }
          label={t('fleet.traffic.errors')}
        />
        <BigStat
          value={`${Math.round(p95)} ms`}
          label={t('fleet.traffic.p95')}
        />
      </div>
      {sum.series.length > 1 ? (
        <Sparkline
          values={sum.series}
          tone="accent"
          width={160}
          fill
          ariaLabel={t('fleet.traffic.spark')}
        />
      ) : null}
    </div>
  )
}
