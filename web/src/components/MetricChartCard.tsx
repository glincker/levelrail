import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type LabelProps,
  type TooltipContentProps,
} from 'recharts'
import {
  chartAccessibleSummary,
  type ChartMarker,
  type ChartRow,
  type ChartUnit,
  formatAxisTick,
  formatMetricValue,
  latestReading,
} from '../lib/metricChart'
import {
  computeNiceYDomain,
  formatPercentWithAdaptivePrecision,
  generateUniqueTicks,
} from '../lib/chartScale'
import { chartPalette } from '../lib/chartPalette'
import type { ResolvedTimeRange } from '../lib/timeRange'
import { Skeleton } from './ui/skeleton'
import { ChartMarkerDot } from './ChartMarkerDot'

// The presentational half of a metric line chart, shared by the app, node
// and database dashboards. Colors are CSS variables (lib/chartPalette.ts).

type SeriesKey = 'primary' | 'secondary' | 'tertiary'

interface SeriesDef {
  key: SeriesKey
  label: string
  color: string
}

function ChartTooltip({
  active,
  payload,
  label,
  unit,
}: TooltipContentProps & { unit: ChartUnit }) {
  if (!active || !payload || payload.length === 0) {
    return null
  }
  return (
    <div className="rounded-md border border-border bg-popover px-2.5 py-1.5 text-xs text-popover-foreground shadow-md tabular-nums">
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

export function MetricChartCard({
  title,
  unit,
  primaryLabel,
  primaryColor,
  secondaryLabel,
  secondaryColor,
  tertiaryLabel,
  tertiaryColor,
  rows,
  range,
  markers = [],
  isLoading,
  error,
  subtitle,
  showPrevious = false,
  previousLabel,
  selectedAt,
  onSelectTime,
  onMarkerClick,
}: {
  title: string
  unit: ChartUnit
  primaryLabel: string
  primaryColor: string
  secondaryLabel?: string
  secondaryColor?: string
  tertiaryLabel?: string
  tertiaryColor?: string
  rows: ChartRow[]
  range: ResolvedTimeRange
  markers?: ChartMarker[]
  isLoading: boolean
  error?: Error | null
  /** Optional small note under the title, e.g. "Summed across 2 containers". */
  subtitle?: string
  /** Draw rows' `previous` values as a dashed muted line. */
  showPrevious?: boolean
  previousLabel?: string
  /** Time of the selected investigation point, drawn as a vertical line. */
  selectedAt?: number
  /** Called with the time under a click on the plot area. */
  onSelectTime?: (t: number) => void
  onMarkerClick?: (marker: ChartMarker) => void
}) {
  const { t } = useTranslation('observability')
  const [hidden, setHidden] = useState<ReadonlySet<SeriesKey>>(new Set())
  const [showMemoryLimit, setShowMemoryLimit] = useState(false)
  const series: SeriesDef[] = [
    { key: 'primary', label: primaryLabel, color: primaryColor },
  ]
  if (secondaryLabel && secondaryColor) {
    series.push({
      key: 'secondary',
      label: secondaryLabel,
      color: secondaryColor,
    })
  }
  if (tertiaryLabel && tertiaryColor) {
    series.push({ key: 'tertiary', label: tertiaryLabel, color: tertiaryColor })
  }
  const spanMs = range.to.getTime() - range.from.getTime()
  const current = isLoading || error ? null : latestReading(rows, unit)
  const toggle = (key: SeriesKey) => {
    setHidden((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  // Compute Y domain for percent and bytes charts with adaptive formatting.
  const yDomain = useMemo(() => {
    if (unit !== 'percent' && unit !== 'bytes') {
      return undefined
    }
    // A memory limit line is hidden by default, so it must not stretch the scale.
    const limitHidden =
      unit === 'bytes' && secondaryLabel === 'Limit' && !showMemoryLimit
    const values = rows
      .flatMap((row) => [
        row.primary,
        limitHidden ? undefined : row.secondary,
        row.tertiary,
      ])
      .filter((v): v is number => v !== undefined)
    return computeNiceYDomain(values, unit)
  }, [rows, unit, secondaryLabel, showMemoryLimit])

  // Compute Y ticks with unique labels for percent charts.
  const yTicks = useMemo(() => {
    if (unit !== 'percent' || !yDomain) {
      return undefined
    }
    return generateUniqueTicks(yDomain, yDomain[1], 'percent')
  }, [unit, yDomain])

  // Check if secondary is a limit line that should be shown as reference.
  const isMemoryChart = unit === 'bytes' && secondaryLabel === 'Limit'
  const shouldShowLimitLine =
    isMemoryChart &&
    showMemoryLimit &&
    rows.some((r) => r.secondary !== undefined)

  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex-1">
          <h3 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <svg
              className="size-2 shrink-0"
              viewBox="0 0 8 8"
              aria-hidden="true"
            >
              <circle cx="4" cy="4" r="4" fill={primaryColor} />
            </svg>
            {title}
          </h3>
          {subtitle ? (
            <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>
          ) : null}
          {isMemoryChart && rows.some((r) => r.secondary !== undefined) ? (
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t('charts.memoryLimit', {
                defaultValue: 'limit {{limit}}, using {{usage}}',
                limit: rows
                  .filter((r) => r.secondary !== undefined)
                  .slice(-1)
                  .map((r) => formatMetricValue('bytes', r.secondary ?? 0))
                  .join(''),
                usage: current,
              })}
            </p>
          ) : null}
        </div>
        {current !== null && !isMemoryChart ? (
          <span className="font-mono text-sm font-medium text-foreground tabular-nums">
            {current}
          </span>
        ) : null}
      </div>
      {isLoading ? (
        <Skeleton className="mt-2 h-56 w-full" />
      ) : error ? (
        <p className="mt-6 text-sm text-destructive">{error.message}</p>
      ) : rows.length === 0 ? (
        <p className="mt-6 text-sm text-muted-foreground">
          {t('chart.noData')}
        </p>
      ) : (
        <>
          <div
            className="mt-2 h-56"
            role="img"
            aria-label={chartAccessibleSummary(rows, unit, title)}
          >
            <ResponsiveContainer width="100%" height="100%">
              <LineChart
                data={rows}
                margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
                onClick={(state) => {
                  const at = Number(state.activeLabel)
                  if (onSelectTime && Number.isFinite(at)) {
                    onSelectTime(at)
                  }
                }}
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
                  domain={yDomain}
                  ticks={yTicks}
                  tickFormatter={(v: number) => {
                    if (unit === 'percent' && yDomain) {
                      return formatPercentWithAdaptivePrecision(v, yDomain[1])
                    }
                    return formatMetricValue(unit, v)
                  }}
                  tick={{ fontSize: 11 }}
                  width={64}
                  stroke="currentColor"
                  className="text-muted-foreground"
                />
                <Tooltip
                  content={(props) => <ChartTooltip {...props} unit={unit} />}
                />
                {markers.map((marker) => (
                  <ReferenceLine
                    key={marker.key}
                    x={marker.t}
                    stroke={marker.color}
                    strokeDasharray="4 4"
                    strokeOpacity={0.7}
                    label={{
                      position: 'insideTop',
                      content: (props: LabelProps) => (
                        <ChartMarkerDot
                          marker={marker}
                          x={Number(props.x ?? 0)}
                          y={Number(props.y ?? 0)}
                          onClick={onMarkerClick}
                        />
                      ),
                    }}
                  />
                ))}
                {selectedAt !== undefined ? (
                  <ReferenceLine
                    x={selectedAt}
                    stroke={chartPalette.selection}
                    strokeWidth={2}
                  />
                ) : null}
                {showPrevious ? (
                  <Line
                    type="monotone"
                    dataKey="previous"
                    name={previousLabel}
                    stroke={chartPalette.muted}
                    strokeDasharray="5 4"
                    dot={false}
                    strokeWidth={1.5}
                    isAnimationActive={false}
                    connectNulls
                  />
                ) : null}
                {series.map((s, i) => {
                  const isSecondary = s.key === 'secondary'
                  const isLimitLine = isMemoryChart && isSecondary
                  const showLine =
                    !hidden.has(s.key) && (!isLimitLine || shouldShowLimitLine)
                  return showLine ? (
                    <Line
                      key={s.key}
                      type="monotone"
                      dataKey={s.key}
                      name={s.label}
                      stroke={s.color}
                      strokeDasharray={
                        i > 1 ? '2 3' : isLimitLine ? '3 3' : undefined
                      }
                      dot={false}
                      strokeWidth={isLimitLine ? 1 : 1.5}
                      isAnimationActive={false}
                      connectNulls
                    />
                  ) : null
                })}
              </LineChart>
            </ResponsiveContainer>
          </div>
          {series.length > 1 || showPrevious || isMemoryChart ? (
            <ul className="mt-2 flex flex-wrap gap-2">
              {series.map((s) => {
                const isLimitLine = isMemoryChart && s.key === 'secondary'
                return (
                  <li key={s.key}>
                    <button
                      type="button"
                      aria-pressed={
                        !hidden.has(s.key) && (!isLimitLine || showMemoryLimit)
                      }
                      onClick={() => {
                        if (isLimitLine) {
                          setShowMemoryLimit(!showMemoryLimit)
                        } else {
                          toggle(s.key)
                        }
                      }}
                      className="inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs text-muted-foreground aria-pressed:text-foreground hover:bg-muted"
                    >
                      <svg
                        className="size-2"
                        viewBox="0 0 8 8"
                        aria-hidden="true"
                      >
                        <circle cx="4" cy="4" r="4" fill={s.color} />
                      </svg>
                      {s.label}
                    </button>
                  </li>
                )
              })}
              {showPrevious && previousLabel ? (
                <li className="inline-flex items-center gap-1.5 px-1.5 py-0.5 text-xs text-muted-foreground">
                  <svg
                    className="h-2 w-4"
                    viewBox="0 0 16 8"
                    aria-hidden="true"
                  >
                    <line
                      x1="0"
                      y1="4"
                      x2="16"
                      y2="4"
                      stroke={chartPalette.muted}
                      strokeDasharray="4 3"
                      strokeWidth="2"
                    />
                  </svg>
                  {previousLabel}
                </li>
              ) : null}
            </ul>
          ) : null}
        </>
      )}
    </section>
  )
}
