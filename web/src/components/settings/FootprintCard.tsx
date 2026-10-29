import { useTranslation } from 'react-i18next'
import { CpuGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import type { IdleFootprint } from '../../queries/controlPlaneHealth'

const STATUS_VARIANT = {
  within_tolerance: 'success',
  over_tolerance: 'destructive',
  tolerance_unset: 'warning',
  no_baseline: 'muted',
} as const

const MIB = 1 << 20

function formatValue(name: string, v: number): string {
  if (name === 'rss') return `${v.toFixed(1)} MB`
  if (name === 'cpu') return `${v.toFixed(2)} %`
  return String(Math.round(v))
}

export function FootprintCard({ idle }: { idle: IdleFootprint }) {
  const { t } = useTranslation('settings', {
    keyPrefix: 'controlPlaneHealth.footprint',
  })
  const { current, baseline, checks } = idle
  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <CpuGlassIcon size="1.2em" aria-hidden="true" />
            {t('title')}
          </CardTitle>
          <Badge variant={STATUS_VARIANT[idle.status]}>
            {t(`status.${idle.status}`)}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground">
          {baseline
            ? t('basis', {
                platform: idle.platform,
                apps: baseline.apps,
                liveApps: current.apps,
              })
            : t('noBaseline', { platform: idle.platform })}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {idle.detail ? (
          <p className="text-sm text-muted-foreground">{idle.detail}</p>
        ) : null}
        {checks.length > 0 ? (
          <ul className="space-y-4">
            {checks.map((c) => (
              <li key={c.name} className="space-y-1">
                <div className="flex flex-wrap items-baseline justify-between gap-2 text-sm">
                  <span className="font-medium">{t(`metric.${c.name}`)}</span>
                  <span className="tabular-nums">
                    <span
                      className={
                        c.ok ? undefined : 'font-medium text-destructive'
                      }
                    >
                      {formatValue(c.name, c.current)}
                    </span>
                    <span className="text-muted-foreground">
                      {' '}
                      {t('vs', {
                        baseline: formatValue(c.name, c.baseline),
                        limit: formatValue(c.name, c.limit),
                      })}
                    </span>
                  </span>
                </div>
                <Progress
                  value={Math.min(100, (c.current / (c.limit || 1)) * 100)}
                  aria-label={t('meterLabel', {
                    metric: t(`metric.${c.name}`),
                  })}
                />
              </li>
            ))}
          </ul>
        ) : (
          <dl className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
            {(
              [
                ['rss', `${(current.rss_bytes / MIB).toFixed(1)} MB`],
                ['cpu', `${current.cpu_percent.toFixed(2)} %`],
                ['goroutines', String(current.goroutines)],
                ['open_fds', String(current.open_fds)],
              ] as const
            ).map(([k, v]) => (
              <div key={k}>
                <dt className="text-xs text-muted-foreground">
                  {t(`metric.${k}`)}
                </dt>
                <dd className="text-lg font-medium tabular-nums">{v}</dd>
              </div>
            ))}
          </dl>
        )}
        <p className="text-xs text-muted-foreground">
          {t('footnote', { seconds: Math.round(current.cpu_window_seconds) })}
        </p>
      </CardContent>
    </Card>
  )
}
