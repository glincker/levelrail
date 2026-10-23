import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CloudArrowDownIcon,
  DatabaseIcon,
  PackageIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Skeleton } from '@/components/ui/skeleton'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MetricTile } from '@/components/kit/MetricTile'
import { SkeletonTile } from '@/components/kit/Skeleton'
import { formatSize } from '@/lib/format'
import {
  useSystemCache,
  type SystemCache,
  type SystemCacheApp,
} from '../../queries/cdnCache'
import { AppPicker } from './AppPicker'
import { CacheReasons, DominantReason } from './CacheReasons'
import { CdnFrame } from './CdnFrame'
import { formatCount, formatHitRate } from './format'
import { HitRateChart } from './HitRateChart'
import { PurgeHistoryTable } from './PurgeHistoryTable'
import { PurgePanel } from './PurgePanel'

const TILE_GRID = 'grid grid-cols-[repeat(auto-fit,minmax(14rem,1fr))] gap-4'

function Tiles({ data }: Readonly<{ data: SystemCache }>) {
  const { t } = useTranslation('cdn')
  const { totals, limits } = data
  const used = limits.max_bytes > 0 ? limits.used_bytes / limits.max_bytes : 0
  const served = totals.hits + totals.stale + totals.misses
  return (
    <div className={TILE_GRID}>
      <MetricTile
        label={t('tiles.hitRate')}
        value={served > 0 ? formatHitRate(totals.hit_rate) : t('tiles.none')}
        series={data.series.map((b) =>
          b.hits + b.misses > 0 ? (b.hits / (b.hits + b.misses)) * 100 : 0,
        )}
        tone="success"
        icon={<ArrowsClockwiseIcon aria-hidden="true" />}
        info={t('tiles.hitRateSince')}
      />
      <MetricTile
        label={t('tiles.saved')}
        value={formatSize(totals.hit_bytes)}
        series={data.series.map((b) => b.hit_bytes)}
        tone="info"
        icon={<CloudArrowDownIcon aria-hidden="true" />}
        info={t('tiles.savedSince')}
      />
      <MetricTile
        label={t('tiles.entries')}
        value={formatCount(totals.entries)}
        icon={<PackageIcon aria-hidden="true" />}
      />
      <MetricTile
        label={t('tiles.memory')}
        value={`${formatSize(limits.used_bytes)} / ${formatSize(limits.max_bytes)}`}
        unit={`${(used * 100).toFixed(0)}%`}
        icon={<DatabaseIcon aria-hidden="true" />}
        info={t('tiles.memoryInfo')}
      />
    </div>
  )
}

function AppsTable({ apps }: Readonly<{ apps: SystemCacheApp[] }>) {
  const { t } = useTranslation('cdn')
  if (apps.length === 0) {
    return (
      <EmptyState
        compact
        icon={<PackageIcon className="size-4" />}
        title={t('apps.emptyTitle')}
        description={t('apps.emptyBody')}
        action={
          <Button
            variant="outline"
            size="sm"
            render={<Link to="/apps" />}
            nativeButton={false}
          >
            {t('apps.emptyAction')}
          </Button>
        }
      />
    )
  }
  const columns: ResponsiveColumn<SystemCacheApp>[] = [
    {
      id: 'app',
      header: t('apps.app'),
      primary: true,
      cell: (a) => (
        <Link
          to="/apps/$name/cache"
          params={{ name: a.app }}
          className="block max-w-56 truncate font-medium underline-offset-4 hover:underline"
          title={a.app}
        >
          {a.app}
        </Link>
      ),
    },
    {
      id: 'cache',
      header: t('apps.cache'),
      priority: 2,
      cell: (a) =>
        a.enabled_domains > 0
          ? t('apps.onFor', { on: a.enabled_domains, total: a.domains })
          : t('apps.off'),
    },
    {
      id: 'requests',
      header: t('apps.requests'),
      align: 'end',
      cell: (a) => (
        <span className="tabular-nums">{formatCount(a.requests)}</span>
      ),
    },
    {
      id: 'hitRate',
      header: t('apps.hitRate'),
      align: 'end',
      cell: (a) => {
        const noHits = a.hits + a.stale === 0
        if (a.requests > 0 && noHits && a.reasons.length > 0) {
          return <DominantReason reasons={a.reasons} />
        }
        return (
          <span className="tabular-nums">
            {a.hits + a.stale + a.misses > 0
              ? formatHitRate(a.hit_rate)
              : t('tiles.none')}
          </span>
        )
      },
    },
    {
      id: 'why',
      header: t('reasons.column'),
      priority: 2,
      cell: (a) => (
        <CacheReasons
          app={a.app}
          reasons={a.reasons}
          suggestPreset={a.suggest_preset}
        />
      ),
    },
    {
      id: 'saved',
      header: t('apps.saved'),
      priority: 3,
      align: 'end',
      cell: (a) => (
        <span className="tabular-nums">{formatSize(a.hit_bytes)}</span>
      ),
    },
  ]
  return (
    <ResponsiveTable
      rows={apps}
      columns={columns}
      getRowId={(a) => a.app}
      ariaLabel={t('apps.aria')}
    />
  )
}

function AppScoped({
  apps,
  children,
}: Readonly<{
  apps: SystemCacheApp[]
  children: (app: string) => React.ReactNode
}>) {
  const { t } = useTranslation('cdn')
  const [app, setApp] = useState(apps[0]?.app ?? '')
  if (apps.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('apps.emptyBody')}</p>
    )
  }
  return (
    <div className="space-y-4">
      <AppPicker apps={apps.map((a) => a.app)} value={app} onChange={setApp} />
      {children(app)}
    </div>
  )
}

function CachesSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div className={TILE_GRID}>
        {[0, 1, 2, 3].map((i) => (
          <SkeletonTile key={i} />
        ))}
      </div>
      <Skeleton className="h-48 rounded-xl" />
      <TableSkeleton columnCount={6} rowCount={4} />
    </div>
  )
}

/** CachesPage is the instance view of the ingress response cache. */
export function CachesPage() {
  const { t } = useTranslation('cdn')
  const { data, error, isPending, refetch } = useSystemCache()

  return (
    <CdnFrame title={t('caches.title')} description={t('caches.description')}>
      <Banner variant="info" title={t('caches.scopeTitle')}>
        {t('caches.scopeBody', { tag: data?.limits.tag_header ?? 'Cache-Tag' })}
      </Banner>
      {isPending ? <CachesSkeleton /> : null}
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
      {data ? (
        <>
          <Tiles data={data} />
          <Card>
            <CardHeader>
              <CardTitle>{t('caches.chartTitle')}</CardTitle>
              <p className="text-sm text-muted-foreground">
                {t('caches.chartHelp')}
              </p>
            </CardHeader>
            <CardContent>
              <HitRateChart series={data.series} />
            </CardContent>
          </Card>
          <Tabs defaultValue="apps">
            <TabsList>
              <TabsTrigger value="apps">{t('caches.tabApps')}</TabsTrigger>
              <TabsTrigger value="purge">{t('caches.tabPurge')}</TabsTrigger>
              <TabsTrigger value="history">
                {t('caches.tabHistory')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="apps" className="pt-2">
              <AppsTable apps={data.apps} />
            </TabsContent>
            <TabsContent value="purge" className="pt-2">
              <Card>
                <CardContent>
                  <AppScoped apps={data.apps}>
                    {(app) => <PurgePanel key={app} app={app} />}
                  </AppScoped>
                </CardContent>
              </Card>
            </TabsContent>
            <TabsContent value="history" className="pt-2">
              <AppScoped apps={data.apps}>
                {(app) => <PurgeHistoryTable key={app} app={app} />}
              </AppScoped>
            </TabsContent>
          </Tabs>
        </>
      ) : null}
    </CdnFrame>
  )
}
