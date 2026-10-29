import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  DownloadGlassIcon,
  GlobeGlassIcon,
  PackageGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MetricTile } from '@/components/kit/MetricTile'
import { SkeletonTile } from '@/components/kit/Skeleton'
import { formatSize } from '@/lib/format'
import {
  useAppCache,
  useApplyCachePreset,
  type AppCache,
  type AppCacheDomain,
} from '../../queries/cdnCache'
import { formatCount, formatHitRate } from './format'
import { HitRateChart } from './HitRateChart'
import { PurgeHistoryTable } from './PurgeHistoryTable'
import { PurgePanel } from './PurgePanel'

const TILE_GRID = 'grid grid-cols-[repeat(auto-fit,minmax(14rem,1fr))] gap-4'

function PresetBanner({
  app,
  data,
}: Readonly<{ app: string; data: AppCache }>) {
  const { t } = useTranslation('cdn')
  const preset = useApplyCachePreset(app)
  const needsPreset = data.domains.some((d) => !d.enabled || d.rules === 0)
  if (!needsPreset) return null
  return (
    <Banner
      variant="info"
      title={t('app.presetTitle')}
      action={
        <Button
          size="sm"
          writeAction
          disabled={preset.isPending}
          onClick={() => preset.mutate(undefined)}
        >
          {preset.isPending ? t('app.presetApplying') : t('app.presetAction')}
        </Button>
      }
    >
      {t('app.presetBody')}
      {preset.isError ? (
        <span className="mt-1 block text-tone-danger">
          {preset.error.message}
        </span>
      ) : null}
    </Banner>
  )
}

function DomainsTable({
  app,
  domains,
}: Readonly<{ app: string; domains: AppCacheDomain[] }>) {
  const { t } = useTranslation('cdn')
  const columns: ResponsiveColumn<AppCacheDomain>[] = [
    {
      id: 'domain',
      header: t('app.domain'),
      primary: true,
      cell: (d) => (
        <span
          className="block max-w-64 truncate font-mono text-xs"
          title={d.domain}
        >
          {d.domain}
        </span>
      ),
    },
    {
      id: 'state',
      header: t('app.state'),
      cell: (d) =>
        d.enabled ? (
          <Badge variant="success">{t('app.on')}</Badge>
        ) : (
          <Badge variant="muted">{t('app.off')}</Badge>
        ),
    },
    {
      id: 'rules',
      header: t('app.rules'),
      priority: 2,
      align: 'end',
      cell: (d) => <span className="tabular-nums">{d.rules}</span>,
    },
    {
      id: 'hitRate',
      header: t('apps.hitRate'),
      align: 'end',
      cell: (d) => (
        <span className="tabular-nums">
          {d.hits + d.stale + d.misses > 0
            ? formatHitRate(d.hit_rate)
            : t('tiles.none')}
        </span>
      ),
    },
    {
      id: 'edit',
      header: <span className="sr-only">{t('app.edit')}</span>,
      align: 'end',
      hideInCard: false,
      cell: (d) => (
        <Link
          to="/domains/$domain"
          params={{ domain: d.domain }}
          search={{ app, tab: 'cache' }}
          className="text-sm underline underline-offset-4"
        >
          {t('app.editRules')}
        </Link>
      ),
    },
  ]
  return (
    <ResponsiveTable
      rows={domains}
      columns={columns}
      getRowId={(d) => d.domain}
      ariaLabel={t('app.domainsAria', { app })}
    />
  )
}

function Body({ app, data }: Readonly<{ app: string; data: AppCache }>) {
  const { t } = useTranslation('cdn')
  const { totals } = data
  if (data.domains.length === 0) {
    return (
      <EmptyState
        icon={<GlobeGlassIcon className="size-5.5" />}
        title={t('app.noDomainsTitle')}
        description={t('app.noDomainsBody')}
        action={
          <Button
            render={<Link to="/apps/$name/domains" params={{ name: app }} />}
            nativeButton={false}
          >
            {t('app.noDomainsAction')}
          </Button>
        }
      />
    )
  }
  return (
    <>
      <PresetBanner app={app} data={data} />
      <div className={TILE_GRID}>
        <MetricTile
          label={t('tiles.hitRate')}
          value={
            totals.hits + totals.stale + totals.misses > 0
              ? formatHitRate(totals.hit_rate)
              : t('tiles.none')
          }
          series={data.series.map((b) =>
            b.hits + b.misses > 0 ? (b.hits / (b.hits + b.misses)) * 100 : 0,
          )}
          tone="success"
          icon={<RefreshCwGlassIcon size="1.2em" aria-hidden="true" />}
          info={t('tiles.hitRateSince')}
        />
        <MetricTile
          label={t('tiles.saved')}
          value={formatSize(totals.hit_bytes)}
          series={data.series.map((b) => b.hit_bytes)}
          tone="info"
          icon={<DownloadGlassIcon size="1.2em" aria-hidden="true" />}
          info={t('tiles.savedSince')}
        />
        <MetricTile
          label={t('tiles.entries')}
          value={formatCount(totals.entries)}
          unit={formatSize(totals.bytes)}
          icon={<PackageGlassIcon size="1.2em" aria-hidden="true" />}
        />
      </div>
      <Card>
        <CardHeader>
          <CardTitle>{t('app.chartTitle')}</CardTitle>
          <p className="text-sm text-muted-foreground">
            {t('app.counts', {
              hits: formatCount(totals.hits + totals.stale),
              misses: formatCount(totals.misses),
              bypasses: formatCount(totals.bypasses),
            })}
          </p>
        </CardHeader>
        <CardContent>
          <HitRateChart series={data.series} />
        </CardContent>
      </Card>
      <DomainsTable app={app} domains={data.domains} />
      <Tabs defaultValue="purge">
        <TabsList>
          <TabsTrigger value="purge">{t('caches.tabPurge')}</TabsTrigger>
          <TabsTrigger value="history">{t('caches.tabHistory')}</TabsTrigger>
        </TabsList>
        <TabsContent value="purge" className="pt-2">
          <Card>
            <CardContent>
              <PurgePanel app={app} />
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="history" className="pt-2">
          <PurgeHistoryTable app={app} />
        </TabsContent>
      </Tabs>
    </>
  )
}

/** AppCachePanel is an app's Cache tab: hit rate, per-domain state, the static assets preset, purge and history. */
export function AppCachePanel({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('cdn')
  const { data, error, isPending, refetch } = useAppCache(app)
  return (
    <div className="space-y-4">
      <p className="max-w-2xl text-sm text-muted-foreground">
        {t('app.subtitle')}
      </p>
      {isPending ? (
        <div className="space-y-4" aria-hidden="true">
          <div className={TILE_GRID}>
            {[0, 1, 2].map((i) => (
              <SkeletonTile key={i} />
            ))}
          </div>
          <Skeleton className="h-48 rounded-xl" />
        </div>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{error.message}</span>
            <Button size="sm" variant="outline" onClick={() => void refetch()}>
              {t('retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {data ? <Body app={app} data={data} /> : null}
    </div>
  )
}
