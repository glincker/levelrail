import { useTranslation } from 'react-i18next'
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { useReducedMotion } from '@/components/kit/useReducedMotion'
import { chartTooltipStyle } from '@/lib/chartPalette'
import {
  FW_OUTCOMES,
  type FwOutcome,
  type FwSeriesPoint,
} from '../../queries/firewallEdge'
import { OUTCOME_STROKE, OUTCOME_SWATCH, formatCount } from './firewallFormat'

const CHART_HEIGHT = 'h-64'

function tickLabel(iso: string, spanHours: number): string {
  const d = new Date(iso)
  return spanHours > 24
    ? d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/** ActionChart plots requests by firewall outcome over time with a legend that doubles as the totals. */
export function ActionChart({
  points,
  totals,
  outcomeLabels,
  chartLabel,
  emptyText,
}: {
  points: FwSeriesPoint[]
  totals: Record<FwOutcome, number>
  outcomeLabels: Record<FwOutcome, string>
  chartLabel: string
  emptyText: string
}) {
  const { t } = useTranslation('firewall')
  const reduced = useReducedMotion()
  const empty = FW_OUTCOMES.every((o) => totals[o] === 0)
  const data = points.map((p) => ({ time: p.time, ...p.counts }))

  return (
    <Card className="min-w-0">
      <ul
        aria-label={t('chart.legend')}
        className="flex flex-wrap gap-x-4 gap-y-1 px-(--card-spacing) text-sm"
      >
        {FW_OUTCOMES.map((o) => (
          <li key={o} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className={`size-2 rounded-full ${OUTCOME_SWATCH[o]}`}
            />
            <span className="text-foreground">{outcomeLabels[o]}</span>
            <span className="text-muted-foreground tabular-nums">
              {totals[o] > 0 ? formatCount(totals[o]) : t('chart.none')}
            </span>
          </li>
        ))}
      </ul>
      <div
        role="img"
        aria-label={chartLabel}
        className={`relative px-(--card-spacing) ${CHART_HEIGHT}`}
      >
        {empty ? (
          <p className="absolute inset-0 z-10 flex items-center justify-center px-6 text-center text-sm text-muted-foreground">
            {emptyText}
          </p>
        ) : null}
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={data}
            margin={{ top: 4, right: 4, bottom: 0, left: -12 }}
          >
            <CartesianGrid stroke="var(--border)" vertical={false} />
            <XAxis
              dataKey="time"
              tickFormatter={(v: string) => tickLabel(v, 24)}
              tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }}
              tickLine={false}
              axisLine={false}
              minTickGap={32}
            />
            <YAxis
              allowDecimals={false}
              tickFormatter={(v: number) => formatCount(v)}
              tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }}
              tickLine={false}
              axisLine={false}
              width={44}
            />
            <Tooltip
              contentStyle={chartTooltipStyle}
              labelFormatter={(v) =>
                typeof v === 'string' ? tickLabel(v, 24) : ''
              }
              formatter={(value, name) => [
                formatCount(Number(value)),
                outcomeLabels[name as FwOutcome] ?? String(name),
              ]}
            />
            {FW_OUTCOMES.map((o) => (
              <Line
                key={o}
                type="monotone"
                dataKey={o}
                stroke={OUTCOME_STROKE[o]}
                strokeWidth={1.75}
                dot={false}
                isAnimationActive={!reduced}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </Card>
  )
}

/** Same footprint as ActionChart. */
export function ActionChartSkeleton() {
  return (
    <Card aria-hidden="true" className="min-w-0">
      <div className="flex gap-4 px-(--card-spacing)">
        {Array.from({ length: 5 }, (_, i) => (
          <Skeleton key={i} className="h-4 w-20" />
        ))}
      </div>
      <div className={`px-(--card-spacing) ${CHART_HEIGHT}`}>
        <Skeleton className="size-full" />
      </div>
    </Card>
  )
}
