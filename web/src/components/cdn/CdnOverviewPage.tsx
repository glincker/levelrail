import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  DownloadGlassIcon,
  GaugeGlassIcon,
  RefreshCwGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { MetricTile } from '@/components/kit/MetricTile'
import { SkeletonTile } from '@/components/kit/Skeleton'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSize } from '@/lib/format'
import {
  useCdnOverview,
  type CdnOverview,
  type CdnWindow,
} from '../../queries/cdnCache'
import { CdnFrame, CdnWindowPicker } from './CdnFrame'
import { formatCount, formatHitRate, formatRate } from './format'
import { HitRateChart } from './HitRateChart'
import { RankedBarList, type RankedBarItem } from './RankedBarList'
import { ThirdPartyCdnNote } from './ThirdPartyCdnNote'

const TILE_GRID = 'grid grid-cols-[repeat(auto-fit,minmax(14rem,1fr))] gap-4'
const CARD_GRID =
  'grid grid-cols-[repeat(auto-fit,minmax(min(100%,24rem),1fr))] gap-4'

function hitRateSeries(o: CdnOverview): number[] {
  return o.cache.series.map((b) =>
    b.hits + b.misses > 0 ? (b.hits / (b.hits + b.misses)) * 100 : 0,
  )
}

function nodeItems(
  o: CdnOverview,
  localName: string,
  appsLabel: (n: number) => string,
): RankedBarItem[] {
  return o.nodes.map((n) => ({
    id: n.id || 'local',
    label: n.name || localName,
    detail: [n.region, appsLabel(n.apps)].filter(Boolean).join(', '),
    share: n.share,
    valueLabel: `${(n.share * 100).toFixed(1)}%`,
  }))
}

function Tiles({ data }: Readonly<{ data: CdnOverview }>) {
  const { t } = useTranslation('cdn')
  return (
    <div className={TILE_GRID}>
      <MetricTile
        label={t('tiles.requests')}
        value={formatCount(data.requests)}
        unit={t('tiles.perSecond', { rate: formatRate(data.rate_per_sec) })}
        series={data.series.map((p) => p.requests)}
        tone="accent"
        icon={<GaugeGlassIcon size="1.4em" aria-hidden="true" />}
        info={t('tiles.requestsInfo')}
      />
      <MetricTile
        label={t('tiles.errors')}
        value={formatHitRate(data.error_rate_5xx)}
        series={data.series.map((p) => p.errors_5xx)}
        tone={data.error_rate_5xx > 0 ? 'danger' : 'success'}
        icon={
          <CircleAlertGlassIcon
            className="text-tone-danger"
            size="1.4em"
            aria-hidden="true"
          />
        }
        info={t('tiles.errorsInfo')}
      />
      <MetricTile
        label={t('tiles.hitRate')}
        value={
          data.cache.hits + data.cache.misses > 0
            ? formatHitRate(data.cache.hit_rate)
            : t('tiles.none')
        }
        series={hitRateSeries(data)}
        tone="success"
        icon={<RefreshCwGlassIcon size="1.4em" aria-hidden="true" />}
        info={t('tiles.hitRateInfo', { minutes: data.cache.covered_minutes })}
      />
      <MetricTile
        label={t('tiles.saved')}
        value={formatSize(data.cache.hit_bytes)}
        series={data.cache.series.map((b) => b.hit_bytes)}
        tone="info"
        icon={<DownloadGlassIcon size="1.4em" aria-hidden="true" />}
        info={t('tiles.savedInfo', { minutes: data.cache.covered_minutes })}
      />
    </div>
  )
}

function ServedFromCard({ data }: Readonly<{ data: CdnOverview }>) {
  const { t } = useTranslation('cdn')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('nodes.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('nodes.help')}</p>
      </CardHeader>
      <CardContent>
        {data.nodes.length === 0 ? (
          <EmptyState
            compact
            icon={<ServerGlassIcon className="size-5" />}
            title={t('nodes.emptyTitle')}
            description={t('nodes.emptyBody')}
            action={
              <Button
                variant="outline"
                size="sm"
                render={<Link to="/domains" />}
                nativeButton={false}
              >
                {t('nodes.emptyAction')}
              </Button>
            }
          />
        ) : (
          <RankedBarList
            ariaLabel={t('nodes.aria')}
            items={nodeItems(data, t('nodes.local'), (n) =>
              t('nodes.apps', { count: n }),
            )}
          />
        )}
        {data.nodes.length > 0 && !data.regions_labeled ? (
          <p className="mt-3 text-xs text-muted-foreground">
            {t('nodes.noRegions')}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}

function CacheCard({ data }: Readonly<{ data: CdnOverview }>) {
  const { t } = useTranslation('cdn')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('cacheCard.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('cacheCard.help')}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <HitRateChart series={data.cache.series} />
        <Button
          variant="outline"
          size="sm"
          render={<Link to="/cdn/caches" />}
          nativeButton={false}
        >
          {t('cacheCard.open')}
        </Button>
      </CardContent>
    </Card>
  )
}

function OverviewSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div className={TILE_GRID}>
        {[0, 1, 2, 3].map((i) => (
          <SkeletonTile key={i} />
        ))}
      </div>
      <div className={CARD_GRID}>
        <Skeleton className="h-64 rounded-xl" />
        <Skeleton className="h-64 rounded-xl" />
      </div>
    </div>
  )
}

/** CdnOverviewPage answers where traffic is served from and how much the cache saves. */
export function CdnOverviewPage() {
  const { t } = useTranslation('cdn')
  const [win, setWin] = useState<CdnWindow>('1h')
  const { data, error, isPending, refetch } = useCdnOverview(win)

  return (
    <CdnFrame
      title={t('overview.title')}
      description={t('overview.description')}
      actions={<CdnWindowPicker value={win} onChange={setWin} />}
    >
      {isPending ? <OverviewSkeleton /> : null}
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
          <div className={CARD_GRID}>
            <ServedFromCard data={data} />
            <CacheCard data={data} />
          </div>
        </>
      ) : null}
      <ThirdPartyCdnNote />
    </CdnFrame>
  )
}
