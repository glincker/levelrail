import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type LabelProps,
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
import type { ResolvedTimeRange } from '../lib/timeRange'
import { Skeleton } from './ui/skeleton'

// The presentational half of a metric line chart: axes, tooltip,
// loading/error/empty states, and optional vertical marker lines drawn
// over the data. Shared by MetricsDashboard.tsx (per-app) and
// NodeMetricsDashboard.tsx (per-node), which differ in where their data
// comes from (different endpoints, different query hooks) but render
// the exact same chart shape once they have rows to show; extracting
// this is what keeps that shape defined in one place instead of two
// near-identical recharts trees drifting apart over time.
//
// Colors are fixed hex values, not Tailwind theme tokens: recharts draws
// to an SVG canvas outside Tailwind's class-based theming, so a color
// that reads fine in both light and dark mode has to be picked directly
// by the caller rather than referenced by class name.

// renderMarkerLabel builds a ReferenceLine `label.content` render
// function (recharts calls this with the line's own computed x/y): a
// small filled dot at the line's top, colored per marker, wrapped in a
// native SVG <title> so hovering the dot shows the marker's tooltip text
// via the browser's own tooltip. No custom tooltip component exists
// elsewhere in this codebase's chart code to reuse, and a native <title>
// needs no extra state, positioning, or dependency to get right.
function renderMarkerLabel(marker: ChartMarker) {
  return function MarkerDot(props: LabelProps) {
    const cx = typeof props.x === 'number' ? props.x : Number(props.x ?? 0)
    const cy = typeof props.y === 'number' ? props.y : Number(props.y ?? 0)
    return (
      <g>
        <title>{marker.tooltip}</title>
        <circle
          cx={cx}
          cy={cy}
          r={4}
          fill={marker.color}
          stroke="currentColor"
          className="text-card"
          strokeWidth={1.5}
        />
      </g>
    )
  }
}

export function MetricChartCard({
  title,
  unit,
  primaryLabel,
  primaryColor,
  secondaryLabel,
  secondaryColor,
  rows,
  range,
  markers = [],
  isLoading,
  error,
  subtitle,
}: {
  title: string
  unit: ChartUnit
  primaryLabel: string
  primaryColor: string
  secondaryLabel?: string
  secondaryColor?: string
  rows: ChartRow[]
  range: ResolvedTimeRange
  markers?: ChartMarker[]
  isLoading: boolean
  error?: Error | null
  /** Optional small note under the title, e.g. "Summed across 2 containers". */
  subtitle?: string
}) {
  const hasSecondary = Boolean(secondaryLabel && secondaryColor)
  const spanMs = range.to.getTime() - range.from.getTime()
  const current = isLoading || error ? null : latestReading(rows, unit)

  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: primaryColor }}
              aria-hidden="true"
            />
            {title}
          </h3>
          {subtitle ? (
            <p className="mt-0.5 text-xs text-muted-foreground">{subtitle}</p>
          ) : null}
        </div>
        {current !== null ? (
          <span className="font-mono text-sm font-medium text-foreground">
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
          No data in this range.
        </p>
      ) : (
        <div
          className="mt-2 h-56"
          role="img"
          aria-label={chartAccessibleSummary(rows, unit, title)}
        >
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={rows}
              margin={{ top: 4, right: 8, left: 0, bottom: 0 }}
            >
              <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
              <XAxis
                dataKey="t"
                type="number"
                domain={['dataMin', 'dataMax']}
                tickFormatter={(t: number) => formatAxisTick(t, spanMs)}
                tick={{ fontSize: 11 }}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <YAxis
                tickFormatter={(v: number) => formatMetricValue(unit, v)}
                tick={{ fontSize: 11 }}
                width={64}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <Tooltip
                labelFormatter={(label) =>
                  new Date(Number(label)).toLocaleString()
                }
                formatter={(value) => formatMetricValue(unit, Number(value))}
              />
              {hasSecondary ? <Legend wrapperStyle={{ fontSize: 11 }} /> : null}
              {markers.map((marker) => (
                <ReferenceLine
                  key={marker.key}
                  x={marker.t}
                  stroke={marker.color}
                  strokeDasharray="4 4"
                  strokeOpacity={0.7}
                  label={{
                    position: 'insideTop',
                    content: renderMarkerLabel(marker),
                  }}
                />
              ))}
              <Line
                type="monotone"
                dataKey="primary"
                name={primaryLabel}
                stroke={primaryColor}
                dot={false}
                strokeWidth={1.5}
                isAnimationActive={false}
                connectNulls
              />
              {hasSecondary ? (
                <Line
                  type="monotone"
                  dataKey="secondary"
                  name={secondaryLabel}
                  stroke={secondaryColor}
                  dot={false}
                  strokeWidth={1.5}
                  isAnimationActive={false}
                  connectNulls
                />
              ) : null}
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </section>
  )
}
