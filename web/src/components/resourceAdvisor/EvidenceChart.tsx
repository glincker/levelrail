import { useTranslation } from 'react-i18next'
import {
  Area,
  AreaChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { chartPalette } from '../../lib/chartPalette'
import { formatBytes, formatNanoCpus } from '../../lib/format'
import type { AdvisorEvidence } from '../../types/resourceAdvisor'

function formatValue(unit: AdvisorEvidence['unit'], v: number): string {
  if (unit === 'nano_cpus') return formatNanoCpus(v)
  if (unit === 'bytes') return formatBytes(v)
  return String(Math.round(v))
}

// Usage against the limit with the p95 line and, when given, the suggested
// value, so the reason for a finding is visible at a glance.
export function EvidenceChart({
  evidence,
  suggested,
}: {
  evidence: AdvisorEvidence
  suggested?: number
}) {
  const { t } = useTranslation('resourceAdvisor')
  const series = evidence.series ?? []
  if (series.length < 2) return null
  const data = series.map((p) => ({ t: Date.parse(p.t), v: p.v }))
  const top = Math.max(
    ...data.map((d) => d.v),
    evidence.limit ?? 0,
    suggested ?? 0,
  )
  const unit = evidence.unit
  return (
    <figure
      className="h-36 w-full"
      aria-label={t('finding.chartLabel', { window: evidence.window })}
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart
          data={data}
          margin={{ top: 6, right: 8, left: 0, bottom: 0 }}
        >
          <XAxis
            dataKey="t"
            type="number"
            domain={['dataMin', 'dataMax']}
            hide
          />
          <YAxis
            domain={[0, top * 1.1]}
            tickFormatter={(v: number) => formatValue(unit, v)}
            tick={{ fontSize: 10 }}
            width={64}
            stroke="currentColor"
            className="text-muted-foreground"
          />
          <Tooltip
            formatter={(v) => [
              formatValue(unit, Number(v)),
              t('finding.usage'),
            ]}
            labelFormatter={(l) => new Date(Number(l)).toLocaleString()}
          />
          <Area
            type="monotone"
            dataKey="v"
            stroke={chartPalette.primary}
            fill={chartPalette.primary}
            fillOpacity={0.15}
            isAnimationActive={false}
            dot={false}
          />
          {evidence.p95 ? (
            <ReferenceLine
              y={evidence.p95}
              stroke={chartPalette.muted}
              strokeDasharray="4 4"
              label={{
                value: t('finding.p95'),
                position: 'insideTopLeft',
                fontSize: 10,
              }}
            />
          ) : null}
          {evidence.limit ? (
            <ReferenceLine
              y={evidence.limit}
              stroke={chartPalette.danger}
              label={{
                value: t('finding.limit'),
                position: 'insideTopRight',
                fontSize: 10,
              }}
            />
          ) : null}
          {suggested ? (
            <ReferenceLine
              y={suggested}
              stroke={chartPalette.success}
              strokeDasharray="6 3"
              label={{
                value: t('finding.suggested'),
                position: 'insideBottomRight',
                fontSize: 10,
              }}
            />
          ) : null}
        </AreaChart>
      </ResponsiveContainer>
    </figure>
  )
}
