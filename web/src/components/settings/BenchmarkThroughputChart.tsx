import { useTranslation } from 'react-i18next'
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { chartSlot, chartTooltipStyle } from '../../lib/chartPalette'

export interface ThroughputPoint {
  label: string
  rps: number
}

// Animation is off so the chart never moves for people who asked for reduced motion.
export default function BenchmarkThroughputChart({
  points,
  title,
}: {
  points: ThroughputPoint[]
  title: string
}) {
  const { t } = useTranslation('resilience')
  const summary = points
    .map((p) => `${p.label}: ${Math.round(p.rps)}`)
    .join(', ')
  return (
    <div
      role="img"
      aria-label={t('benchmarks.chart.aria', { title, summary })}
      className="h-48 w-full"
    >
      <ResponsiveContainer width="100%" height="100%">
        <BarChart
          data={points}
          margin={{ top: 8, right: 8, bottom: 0, left: 0 }}
        >
          <CartesianGrid vertical={false} stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 11 }} tickLine={false} />
          <YAxis tick={{ fontSize: 11 }} tickLine={false} width={48} />
          <Tooltip
            contentStyle={chartTooltipStyle}
            formatter={(v) => [`${Math.round(Number(v))} req/s`, title]}
          />
          <Bar
            dataKey="rps"
            fill={chartSlot.requests}
            radius={[3, 3, 0, 0]}
            isAnimationActive={false}
          />
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}
