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
import { formatMoney } from '../../lib/usageFormat'
import type { CostExploration } from '../../types/costExplorer'
import { seriesLabel } from './costLabels'

// Static class names so Tailwind generates them; index matches seriesColor.
const SWATCH = [
  'bg-[var(--chart-series-1)]',
  'bg-[var(--chart-series-2)]',
  'bg-[var(--chart-series-3)]',
  'bg-[var(--chart-series-4)]',
  'bg-[var(--chart-series-5)]',
  'bg-[var(--chart-series-6)]',
] as const

// Cost per day or month, stacked by the chosen group.
export function CostTrendChart({ data: res }: { data: CostExploration }) {
  const { t } = useTranslation('costs')
  if (res.series.length === 0 || res.buckets.length === 0) return null
  const rows = res.buckets.map((bucket, i) => {
    const row: Record<string, number | string> = { bucket }
    for (const s of res.series) row[s.key] = s.values[i] ?? 0
    return row
  })
  const colorAt = (i: number, key: string) =>
    key === 'other' ? chartMuted : seriesColor(i)
  return (
    <figure className="space-y-3" aria-label={t('chart.label')}>
      <div className="h-64 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <BarChart
            data={rows}
            margin={{ top: 6, right: 8, left: 0, bottom: 0 }}
          >
            <XAxis
              dataKey="bucket"
              tick={{ fontSize: 10 }}
              tickFormatter={(d: string) =>
                res.granularity === 'daily' ? d.slice(5) : d
              }
              minTickGap={16}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <YAxis
              tick={{ fontSize: 10 }}
              width={56}
              tickFormatter={(v: number) => formatMoney(v, res.currency)}
              stroke="currentColor"
              className="text-muted-foreground"
            />
            <Tooltip
              formatter={(v, name) => [
                formatMoney(Number(v), res.currency),
                String(name),
              ]}
            />
            {res.series.map((s, i) => (
              <Bar
                key={s.key}
                dataKey={s.key}
                name={seriesLabel(res, s, t)}
                stackId="cost"
                fill={colorAt(i, s.key)}
                isAnimationActive={false}
              />
            ))}
          </BarChart>
        </ResponsiveContainer>
      </div>
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {res.series.map((s, i) => (
          <li key={s.key} className="flex items-center gap-1.5">
            <span
              aria-hidden="true"
              className={`size-2.5 rounded-sm ${s.key === 'other' ? 'bg-[var(--chart-muted)]' : SWATCH[i % SWATCH.length]}`}
            />
            {seriesLabel(res, s, t)}
          </li>
        ))}
      </ul>
    </figure>
  )
}
