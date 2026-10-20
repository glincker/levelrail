import { InfoTip, MetricTile, SkeletonTile, StatusPill } from '@/components/kit'
import {
  formatEngineMetric,
  healthLabel,
  healthTone,
  seriesTone,
  visibleSeries,
} from '../../lib/engineMetrics'
import { useModelEngineMetrics } from '../../queries/modelEngineMetrics'
import type { EngineMetricSeries } from '../../types/models'

const WINDOW_HOURS = 1

function UnavailableTile({
  series,
  engine,
}: {
  series: EngineMetricSeries
  engine: string
}) {
  return (
    <div
      data-testid="metric-unavailable"
      className="flex flex-col gap-2 rounded-xl border border-dashed border-border p-4"
    >
      <span className="text-xs text-muted-foreground">{series.label}</span>
      <span className="text-sm text-muted-foreground">
        Not available for {engine}
      </span>
    </div>
  )
}

function SeriesTile({ series }: { series: EngineMetricSeries }) {
  const shown =
    series.latest === null
      ? { value: 'no data' }
      : formatEngineMetric(series.latest, series.unit)
  return (
    <MetricTile
      label={series.label}
      value={shown.value}
      unit={shown.unit}
      tone={seriesTone(series)}
      series={
        series.points.length > 1 ? series.points.map((p) => p.v) : undefined
      }
    />
  )
}

export function ModelEnginePanel({
  modelName,
  engine,
}: {
  modelName: string
  engine: string
}) {
  const { data, isPending, error } = useModelEngineMetrics(
    modelName,
    WINDOW_HOURS,
  )
  return (
    <section aria-label="Engine metrics" className="space-y-3">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">Engine metrics</h3>
        {data ? (
          <StatusPill
            tone={healthTone(data.health)}
            label={healthLabel(data.health)}
            title={data.health.summary}
          />
        ) : null}
        {data ? (
          <InfoTip label="About engine metrics">{data.note}</InfoTip>
        ) : null}
      </div>
      {isPending ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <SkeletonTile />
          <SkeletonTile />
          <SkeletonTile />
        </div>
      ) : error ? (
        <p role="alert" className="text-sm text-destructive">
          {error.message}
        </p>
      ) : (
        <>
          {data.health.reasons.map((r) => (
            <p key={r} className="text-sm text-muted-foreground">
              {r}
            </p>
          ))}
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {visibleSeries(data.series).map((s) =>
              s.supported ? (
                <SeriesTile key={s.id} series={s} />
              ) : (
                <UnavailableTile key={s.id} series={s} engine={engine} />
              ),
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            Last hour, scraped from the engine every few seconds.
          </p>
        </>
      )}
    </section>
  )
}
