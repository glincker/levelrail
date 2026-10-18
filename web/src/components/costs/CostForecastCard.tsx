import { useTranslation } from 'react-i18next'
import {
  Area,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { chartPalette } from '../../lib/chartPalette'
import { formatMoney } from '../../lib/usageFormat'
import type { CostConfidence, CostExploration } from '../../types/costExplorer'
import { forecastPoints } from './forecastPoints'

const CONFIDENCE_VARIANT: Record<
  CostConfidence,
  'success' | 'info' | 'warning'
> = { high: 'success', medium: 'info', low: 'warning' }

function daysInMonthOf(iso: string): number {
  const d = new Date(iso)
  return new Date(
    Date.UTC(d.getUTCFullYear(), d.getUTCMonth() + 1, 0),
  ).getUTCDate()
}

// Month end forecast with its range, from the month to date daily series.
export function CostForecastCard({ data }: { data: CostExploration }) {
  const { t } = useTranslation('costs')
  const f = data.forecast
  if (!f) return null
  const points = forecastPoints(data.trend, daysInMonthOf(data.period.from), f)
  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{t('forecast.title')}</CardTitle>
        <Badge variant={CONFIDENCE_VARIANT[f.confidence]}>
          {t(`forecast.confidence.${f.confidence}`)}
        </Badge>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-2xl font-semibold tabular-nums">
          {formatMoney(f.mid, data.currency)}
        </p>
        <p className="text-sm text-muted-foreground">
          {t('forecast.range', {
            low: formatMoney(f.low, data.currency),
            high: formatMoney(f.high, data.currency),
          })}
        </p>
        <p className="text-xs text-muted-foreground">{t('forecast.hint')}</p>
        <div
          className="h-44 w-full"
          role="img"
          aria-label={t('forecast.chart')}
        >
          <ResponsiveContainer width="100%" height="100%">
            <ComposedChart
              data={points}
              margin={{ top: 6, right: 8, left: 0, bottom: 0 }}
            >
              <XAxis
                dataKey="day"
                tick={{ fontSize: 10 }}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <YAxis
                tick={{ fontSize: 10 }}
                width={56}
                tickFormatter={(v: number) => formatMoney(v, data.currency)}
                stroke="currentColor"
                className="text-muted-foreground"
              />
              <Tooltip
                formatter={(v) =>
                  Array.isArray(v)
                    ? `${formatMoney(Number(v[0]), data.currency)} - ${formatMoney(Number(v[1]), data.currency)}`
                    : formatMoney(Number(v), data.currency)
                }
              />
              <Area
                dataKey="band"
                name={t('forecast.bandName')}
                stroke="none"
                fill={chartPalette.neutral}
                fillOpacity={0.25}
                isAnimationActive={false}
              />
              <Line
                dataKey="actual"
                name={t('forecast.actual')}
                stroke={chartPalette.primary}
                dot={false}
                isAnimationActive={false}
              />
              <Line
                dataKey="mid"
                name={t('forecast.projected')}
                stroke={chartPalette.primary}
                strokeDasharray="4 3"
                dot={false}
                isAnimationActive={false}
              />
            </ComposedChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  )
}
