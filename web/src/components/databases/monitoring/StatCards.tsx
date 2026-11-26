import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { formatMetricValue } from '../../../lib/metricChart'
import { isCollected, type ChartDef, type SeriesMap } from './chartRegistry'
import { deltaVsPrevious, lastValue, sparklinePath } from './monitoringStats'

function Stat({
  def,
  data,
  loading,
}: {
  def: ChartDef
  data: SeriesMap
  loading: boolean
}) {
  const { t } = useTranslation('dbMonitoring')
  const primary = def.series[0]
  const series = primary ? data[primary.metric] : undefined
  const last = lastValue(series?.points)
  const delta = deltaVsPrevious(series?.points, series?.previous_points)
  const limit =
    def.id === 'memory' ? lastValue(data.memory_limit_bytes?.points) : null
  const path = sparklinePath(series?.points)
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2">
      <p className="text-xs text-muted-foreground">
        {t(`charts.${def.titleKey}`)}
      </p>
      {loading && last === null ? (
        <Skeleton className="mt-1 h-7 w-24" />
      ) : (
        <>
          <p className="mt-0.5 flex items-baseline gap-1.5 tabular-nums">
            <span className="text-lg font-semibold text-foreground">
              {last === null
                ? t('stats.none')
                : formatMetricValue(def.unit, last)}
            </span>
            {limit !== null && limit > 0 ? (
              <span className="text-xs text-muted-foreground">
                {t('stats.ofLimit', {
                  limit: formatMetricValue('bytes', limit),
                })}
              </span>
            ) : null}
          </p>
          <div className="mt-1 flex h-6 items-center justify-between gap-2">
            {path ? (
              <svg
                viewBox="0 0 100 24"
                preserveAspectRatio="none"
                className="h-6 w-20 text-primary"
                aria-hidden="true"
              >
                <polyline
                  points={path}
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="1.5"
                  vectorEffect="non-scaling-stroke"
                />
              </svg>
            ) : (
              <span />
            )}
            <span className="text-xs text-muted-foreground tabular-nums">
              {delta === null
                ? t('stats.noPrevious')
                : t('stats.delta', {
                    sign: delta >= 0 ? '+' : '-',
                    value: Math.abs(delta).toFixed(
                      Math.abs(delta) < 10 ? 1 : 0,
                    ),
                  })}
            </span>
          </div>
        </>
      )}
    </div>
  )
}

export function StatCards({
  defs,
  data,
  loading,
}: {
  defs: readonly ChartDef[]
  data: SeriesMap
  loading: boolean
}) {
  const { t } = useTranslation('dbMonitoring')
  const stats = defs.filter(isCollected)
  return (
    <ul
      aria-label={t('stats.label')}
      className="grid grid-cols-2 gap-3 lg:grid-cols-4"
    >
      {stats.map((def) => (
        <li key={def.id}>
          <Stat def={def} data={data} loading={loading} />
        </li>
      ))}
    </ul>
  )
}
