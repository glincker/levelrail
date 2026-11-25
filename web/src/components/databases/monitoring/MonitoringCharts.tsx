import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipContentProps,
} from 'recharts'
import { ActivityGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { computeNiceYDomain } from '../../../lib/chartScale'
import {
  formatAxisTick,
  formatMetricValue,
  type ChartUnit,
} from '../../../lib/metricChart'
import type { ResolvedTimeRange } from '../../../lib/timeRange'
import {
  buildRows,
  isCollected,
  type ChartDef,
  type SeriesMap,
} from './chartRegistry'
import type { MonitoringState } from './monitoringState'
import { isInactive } from './monitoringState'

// Lazy chunk: recharts stays out of the first load of the database routes.

const HATCH =
  'bg-[repeating-linear-gradient(135deg,transparent_0_6px,var(--border)_6px_7px)]'

function Tip({
  active,
  payload,
  label,
  unit,
}: TooltipContentProps & { unit: ChartUnit }) {
  if (!active || !payload?.length) return null
  return (
    <div className="rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs text-popover-foreground tabular-nums shadow-md">
      <p className="mb-1 font-medium">
        {new Date(Number(label)).toLocaleString()}
      </p>
      {payload.map((p) => (
        <p key={String(p.dataKey)} className="flex justify-between gap-4">
          <span className="text-muted-foreground">{p.name}</span>
          <span>{formatMetricValue(unit, Number(p.value))}</span>
        </p>
      ))}
    </div>
  )
}

function Hatched({
  title,
  body,
  tone = 'muted',
}: {
  title: string
  body: string
  tone?: 'muted' | 'danger'
}) {
  return (
    <div
      className={cn(
        'flex h-44 flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border px-4 text-center',
        HATCH,
      )}
    >
      <p
        className={cn(
          'rounded bg-card px-2 text-sm font-medium',
          tone === 'danger' ? 'text-destructive' : 'text-foreground',
        )}
      >
        {title}
      </p>
      <p className="rounded bg-card px-2 text-xs text-muted-foreground">
        {body}
      </p>
    </div>
  )
}

function ChartCard({
  def,
  data,
  range,
  state,
  isLoading,
  syncId,
}: {
  def: ChartDef
  data: SeriesMap
  range: ResolvedTimeRange
  state: MonitoringState
  isLoading: boolean
  syncId: string
}) {
  const { t } = useTranslation('dbMonitoring')
  const [shown, setShown] = useState<ReadonlySet<number>>(
    () => new Set(def.series.flatMap((s, i) => (s.defaultHidden ? [] : [i]))),
  )
  const rows = useMemo(() => buildRows(def, data), [def, data])
  const title = t(`charts.${def.titleKey}`)
  const spanMs = range.to.getTime() - range.from.getTime()
  const domain = useMemo(() => {
    if (def.unit !== 'percent' && def.unit !== 'bytes') return undefined
    const vals = rows.flatMap((r) =>
      def.series.flatMap((_, i) =>
        shown.has(i) && r[`s${i}`] !== undefined ? [r[`s${i}`] as number] : [],
      ),
    )
    return computeNiceYDomain(vals, def.unit)
  }, [rows, def, shown])

  const toggle = (i: number) => {
    setShown((prev) => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })
  }

  let body
  if (!isCollected(def)) {
    body = (
      <Hatched
        title={t('states.notCollected.title')}
        body={t('states.notCollected.body')}
      />
    )
  } else if (isInactive(state)) {
    body = (
      <Hatched
        title={t(`states.${state}.title`)}
        body={t(`states.${state}.chart`)}
        tone={state === 'failed' ? 'danger' : 'muted'}
      />
    )
  } else if (isLoading && rows.length === 0) {
    body = <Skeleton className="h-44 w-full" />
  } else if (rows.length === 0) {
    body = (
      <Hatched
        title={t('states.noData.title')}
        body={t('states.noData.chart')}
      />
    )
  } else {
    body = (
      <div
        className="h-44"
        role="img"
        aria-label={t('chartLabel', { title, count: rows.length })}
      >
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={rows}
            syncId={syncId}
            syncMethod="value"
            margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
          >
            <CartesianGrid
              strokeDasharray="3 3"
              stroke="var(--border)"
              strokeOpacity={0.6}
            />
            <XAxis
              dataKey="t"
              type="number"
              domain={[range.from.getTime(), range.to.getTime()]}
              allowDataOverflow
              tickFormatter={(v: number) => formatAxisTick(v, spanMs)}
              tick={{ fontSize: 11 }}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <YAxis
              domain={domain}
              tickFormatter={(v: number) => formatMetricValue(def.unit, v)}
              tick={{ fontSize: 11 }}
              width={56}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <Tooltip content={(p) => <Tip {...p} unit={def.unit} />} />
            {def.series.map((s, i) =>
              shown.has(i) ? (
                <Line
                  key={s.metric}
                  type="monotone"
                  dataKey={`s${i}`}
                  name={t(`series.${s.labelKey}`)}
                  stroke={s.color}
                  strokeWidth={1.5}
                  strokeDasharray={s.defaultHidden ? '3 3' : undefined}
                  dot={false}
                  isAnimationActive={false}
                  connectNulls
                />
              ) : null,
            )}
          </LineChart>
        </ResponsiveContainer>
      </div>
    )
  }

  return (
    <section
      aria-label={title}
      className="rounded-lg border border-border bg-card p-3"
    >
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-medium text-foreground">
          <ActivityGlassIcon
            className="size-4 text-muted-foreground"
            aria-hidden="true"
          />
          {title}
        </h3>
        {isCollected(def) && def.series.length > 1 ? (
          <ul className="flex flex-wrap gap-1">
            {def.series.map((s, i) => (
              <li key={s.metric}>
                <button
                  type="button"
                  aria-pressed={shown.has(i)}
                  onClick={() => {
                    toggle(i)
                  }}
                  className="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-muted aria-pressed:text-foreground"
                >
                  <svg className="size-2" viewBox="0 0 8 8" aria-hidden="true">
                    <circle cx="4" cy="4" r="4" fill={s.color} />
                  </svg>
                  {t(`series.${s.labelKey}`)}
                </button>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
      {body}
    </section>
  )
}

export default function MonitoringCharts({
  defs,
  data,
  range,
  state,
  isLoading,
  syncId,
}: {
  defs: readonly ChartDef[]
  data: SeriesMap
  range: ResolvedTimeRange
  state: MonitoringState
  isLoading: boolean
  syncId: string
}) {
  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
      {defs.map((def) => (
        <ChartCard
          key={def.id}
          def={def}
          data={data}
          range={range}
          state={state}
          isLoading={isLoading}
          syncId={syncId}
        />
      ))}
    </div>
  )
}
