import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BroomIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  useDomainCacheStats,
  usePurgeCache,
} from '../../queries/domainPolicies'
import type { CacheBucket } from '../../queries/domainPolicyTypes'
import { FieldMessage, NativeSelect, Section } from './PolicyShared'

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
  const purge = usePurgeCache(app, domain)
  const [scope, setScope] = useState<'url' | 'prefix' | 'all'>('url')
  const [value, setValue] = useState('')
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
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          purge.mutate(scope === 'all' ? { scope } : { scope, value })
        }}
      >
        <NativeSelect
          id="purge-scope"
          label={t('cache.purgeScope')}
          value={scope}
          options={[
            { value: 'url', label: t('cache.purgeUrl') },
            { value: 'prefix', label: t('cache.purgePrefix') },
            { value: 'all', label: t('cache.purgeAll') },
          ]}
          onChange={(v) => setScope(v as 'url' | 'prefix' | 'all')}
        />
        {scope !== 'all' ? (
          <label className="flex flex-1 flex-col gap-1 text-xs">
            <span className="text-muted-foreground">
              {t('cache.purgeValue')}
            </span>
            <Input
              value={value}
              placeholder="/blog/post?id=1"
              onChange={(e) => setValue(e.target.value)}
            />
          </label>
        ) : null}
        <Button
          type="submit"
          size="sm"
          variant="outline"
          disabled={purge.isPending}
        >
          <BroomIcon />
          {t('cache.purge')}
        </Button>
      </form>
      {purge.isSuccess ? (
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {t('cache.purged', { count: purge.data.purged })}
        </p>
      ) : null}
      <FieldMessage message={purge.error?.message} />
    </Section>
  )
}
