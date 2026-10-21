import { useTranslation } from 'react-i18next'
import { TimerIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { BuildSpeedAdvice } from './BuildSpeedAdvice'
import { formatBuildSeconds } from '../lib/buildDuration'
import { useBuildStats, type BuildRun } from '../queries/buildPerformance'

const BAR_W = 6
const BAR_GAP = 2
const CHART_H = 40

function DurationBars({ runs, label }: { runs: BuildRun[]; label: string }) {
  const ordered = [...runs].reverse()
  const max = Math.max(1, ...ordered.map((r) => r.duration_seconds))
  const width = ordered.length * (BAR_W + BAR_GAP)
  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${width} ${CHART_H}`}
      className="h-10 w-full max-w-md"
      preserveAspectRatio="none"
    >
      {ordered.map((r, i) => {
        const h = Math.max(2, (Math.max(0, r.duration_seconds) / max) * CHART_H)
        return (
          <rect
            key={`${r.started_at}-${i}`}
            x={i * (BAR_W + BAR_GAP)}
            y={CHART_H - h}
            width={BAR_W}
            height={h}
            rx={1}
            className={
              r.status === 'success' ? 'fill-primary' : 'fill-destructive'
            }
          />
        )
      })}
    </svg>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-semibold tabular-nums">{value}</dd>
    </div>
  )
}

export function BuildTrendCard({ appName }: { appName: string }) {
  const { t } = useTranslation('buildPerformance')
  const { data, isPending, error } = useBuildStats(appName)
  if (isPending) return <CardSkeleton titleWidth="w-32" rows={2} />

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TimerIcon aria-hidden="true" className="size-4" />
          {t('trend.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? (
          <p role="alert" className="text-sm text-muted-foreground">
            {t('trend.loadError')}
          </p>
        ) : null}
        {data && data.trend.builds === 0 ? (
          <p className="text-sm text-muted-foreground">{t('trend.empty')}</p>
        ) : null}
        {data && data.trend.builds > 0 ? (
          <>
            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Stat
                label={t('trend.p50')}
                value={
                  data.trend.succeeded > data.trend.skewed
                    ? formatBuildSeconds(data.trend.p50_seconds)
                    : t('trend.none')
                }
              />
              <Stat
                label={t('trend.p95')}
                value={
                  data.trend.succeeded > data.trend.skewed
                    ? formatBuildSeconds(data.trend.p95_seconds)
                    : t('trend.none')
                }
              />
              <Stat
                label={t('trend.cacheHit')}
                value={
                  data.trend.cache_known
                    ? `${Math.round(data.trend.cache_hit_ratio * 100)}%`
                    : t('trend.none')
                }
              />
              <Stat
                label={t('trend.failures')}
                value={String(data.trend.failed + data.trend.timed_out)}
              />
            </dl>
            <DurationBars
              runs={data.runs}
              label={t('trend.chartLabel', { count: data.runs.length })}
            />
            <p className="text-xs text-muted-foreground">
              {t('trend.footnote', { count: data.trend.builds })}
              {data.trend.skewed > 0
                ? ` ${t('trend.skewed', { count: data.trend.skewed })}`
                : ''}
            </p>
            <BuildSpeedAdvice appName={appName} />
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}
