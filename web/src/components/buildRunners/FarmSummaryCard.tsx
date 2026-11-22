import { useTranslation } from 'react-i18next'
import { useBuildFarmSummary } from '../../queries/buildRunners'

const CHART_W = 240
const CHART_H = 40

function Stat({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-semibold tabular-nums text-foreground">
        {value}
      </dd>
    </div>
  )
}

// One card that sizes the build farm: throughput, queue wait, cache and slot use.
export function FarmSummaryCard() {
  const { t } = useTranslation('buildCenter')
  const { data } = useBuildFarmSummary()
  const buckets = data && Array.isArray(data.buckets) ? data.buckets : []
  if (!data || buckets.length === 0 || data.builds === 0) return null

  const scale = Math.max(data.peak_slots, data.slots_total, 1)
  const barW = CHART_W / buckets.length
  const peak = data.peak_slots.toFixed(1)
  const sec = (v: number) => t('farm.seconds', { value: Math.round(v) })

  return (
    <section
      aria-label={t('farm.title', { hours: data.hours })}
      data-testid="farm-summary"
      className="flex flex-wrap items-center gap-x-6 gap-y-2 rounded-lg border border-border px-3 py-2"
    >
      <div className="min-w-0">
        <h3 className="text-xs font-semibold text-muted-foreground">
          {t('farm.title', { hours: data.hours })}
        </h3>
        <p className="text-sm text-foreground">
          {t('farm.builds', {
            count: data.builds,
            perHour: data.builds_per_hour.toFixed(1),
          })}
        </p>
      </div>
      <dl className="flex flex-wrap gap-x-6 gap-y-1">
        <Stat label={t('farm.waitP50')} value={sec(data.wait_p50_seconds)} />
        <Stat label={t('farm.waitP95')} value={sec(data.wait_p95_seconds)} />
        <Stat
          label={t('farm.cache')}
          value={
            data.cache_known
              ? `${Math.round(data.cache_hit_ratio * 100)}%`
              : t('farm.cacheUnknown')
          }
        />
        <Stat
          label={t('farm.peak')}
          value={t('farm.slotsOf', { peak, total: data.slots_total })}
        />
      </dl>
      <svg
        role="img"
        aria-label={t('farm.chart', { peak })}
        viewBox={`0 0 ${CHART_W} ${CHART_H}`}
        className="h-10 w-48 text-primary"
      >
        {buckets.map((b, i) => {
          const h = Math.max(
            (b.avg_slots / scale) * CHART_H,
            b.avg_slots > 0 ? 1 : 0,
          )
          return (
            <rect
              key={b.start}
              x={i * barW + 0.5}
              y={CHART_H - h}
              width={Math.max(barW - 1, 1)}
              height={h}
              fill="currentColor"
              opacity={0.8}
            />
          )
        })}
      </svg>
    </section>
  )
}
