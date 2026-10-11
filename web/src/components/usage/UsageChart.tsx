import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { chartMuted, seriesColor } from '../../lib/chartPalette'
import { formatMoney, stackSeries } from '../../lib/usageFormat'
import type { UsageReport } from '../../types/usage'

const MAX_SERIES = 5

// Static class names so Tailwind generates them; index matches seriesColor.
const SWATCH = [
  'bg-[var(--chart-series-1)]',
  'bg-[var(--chart-series-2)]',
  'bg-[var(--chart-series-3)]',
  'bg-[var(--chart-series-4)]',
  'bg-[var(--chart-series-5)]',
  'bg-[var(--chart-series-6)]',
] as const

// Daily cost stacked by project, top projects named and the rest folded.
export function UsageChart({ report }: { report: UsageReport }) {
  const { t } = useTranslation('usage')
  const series = stackSeries(report, MAX_SERIES, {
    other: t('chart.other'),
    unassigned: t('chart.unassigned'),
  })
  if (series.length === 0 || report.days.length === 0) return null
  const data = report.days.map((day, i) => {
    const row: Record<string, number | string> = { day }
    for (const s of series) row[s.key] = s.values[i] ?? 0
    return row
  })
  const colorAt = (i: number, key: string) =>
    key === '__other' ? chartMuted : seriesColor(i)
  return (
    <figure className="space-y-3" aria-label={t('chart.label')}>
      <div className="h-56 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart
            data={data}
            margin={{ top: 6, right: 8, left: 0, bottom: 0 }}
          >
            <XAxis
              dataKey="day"
              tick={{ fontSize: 10 }}
              tickFormatter={(d: string) => d.slice(5)}
              minTickGap={16}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <YAxis
              tick={{ fontSize: 10 }}
              width={52}
              tickFormatter={(v: number) => formatMoney(v, report.currency)}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <Tooltip
              formatter={(v, name) => [
                formatMoney(Number(v), report.currency),
                String(name),
              ]}
            />
            {series.map((s, i) => (
              <Bar
                key={s.key}
                dataKey={s.key}
                name={s.label}
                stackId="cost"
                fill={colorAt(i, s.key)}
                isAnimationActive={false}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {series.map((s, i) => (
          <li key={s.key} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className={`size-2.5 rounded-sm ${s.key === '__other' ? 'bg-[var(--chart-muted)]' : SWATCH[i % SWATCH.length]}`}
            />
            {s.label}
          </li>
        ))}
      </ul>
    </figure>
  )
}
