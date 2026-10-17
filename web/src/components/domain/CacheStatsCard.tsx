import { useTranslation } from 'react-i18next'
import { useDomainCacheStats } from '../../queries/domainPolicies'
import type { CacheBucket } from '../../queries/domainPolicyTypes'
import { PurgePanel } from '../cdn/PurgePanel'
import { Section } from './PolicyShared'

// Sparkline draws hits (solid) and misses (muted) per minute in a fixed
// viewBox, so it scales with its container without inline styles.
export function Sparkline({
  series,
  label,
}: {
  series: CacheBucket[]
  label: string
}) {
  const max = Math.max(1, ...series.map((b) => Math.max(b.hits, b.misses)))
  const w = 120
  const h = 28
  const step = series.length > 1 ? w / (series.length - 1) : w
  const line = (pick: (b: CacheBucket) => number) =>
    series
      .map(
        (b, i) =>
          `${(i * step).toFixed(1)},${(h - (pick(b) / max) * h).toFixed(1)}`,
      )
      .join(' ')
  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      className="h-8 w-40"
      role="img"
      aria-label={label}
      preserveAspectRatio="none"
    >
      <polyline
        points={line((b) => b.misses)}
        className="fill-none stroke-muted-foreground"
        strokeWidth="1"
      />
      <polyline
        points={line((b) => b.hits)}
        className="fill-none stroke-primary"
        strokeWidth="1.5"
      />
    </svg>
  )
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`
  return `${(n / 1024 / 1024).toFixed(1)} MiB`
}

export function CacheStatsCard({
  app,
  domain,
}: {
  app: string
  domain: string
}) {
  const { t } = useTranslation('domainPolicies')
  const { data: stats } = useDomainCacheStats(app, domain)
  const total = stats ? stats.hits + stats.misses : 0
  const ratio = stats && total > 0 ? Math.round((stats.hits / total) * 100) : 0

  return (
    <Section title={t('cache.statsTitle')} description={t('cache.xcacheHelp')}>
      {stats ? (
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span>{t('cache.hitRatio', { ratio })}</span>
          <span>
            {t('cache.counts', {
              hits: stats.hits,
              misses: stats.misses,
              bypasses: stats.bypasses,
            })}
          </span>
          <span>
            {t('cache.size', {
              entries: stats.entries,
              bytes: formatBytes(stats.bytes),
            })}
          </span>
          <Sparkline series={stats.series} label={t('cache.sparkline')} />
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t('cache.noStats')}</p>
      )}
      <PurgePanel app={app} domain={domain} />
    </Section>
  )
}
