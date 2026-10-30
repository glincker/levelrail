import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { DeployMarkerDetails } from '../DeployMarkerDetails'
import { RequestMetricsSection } from '../RequestMetricsSection'
import { deployAttemptsQueryOptions } from '../../queries/deployAttempts'
import { useTrafficDetail } from '../../queries/trafficDetail'
import { deployMarkers } from '../../lib/chartMarkers'
import type { ChartMarker } from '../../lib/metricChart'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { AbuseCounters } from '../../types/abuse'
import type { TrafficDetail, TrafficWindow } from '../../types/trafficDetail'
import { formatMs, formatPercent, formatRate } from './format'
import { AbuseDefenseCard } from './AbuseDefenseCard'
import { ErrorSpikesCard } from './ErrorSpikesCard'
import { ShieldBlockedCard } from './ShieldBlockedCard'
import {
  AudienceTable,
  ReplicaTable,
  SampleList,
  SlowRouteTable,
  TopRoutesCard,
} from './TrafficTables'

const WINDOWS: TrafficWindow[] = ['5m', '1h', '24h', '7d']

const NO_ABUSE: AbuseCounters = {
  allowed: 0,
  would_block: 0,
  blocked: 0,
  fail_open: 0,
  by_reason: [],
}

function Kpi({
  label,
  value,
  note,
  tone,
}: Readonly<{ label: string; value: string; note?: string; tone?: 'bad' }>) {
  return (
    <Card>
      <CardContent className="space-y-1 py-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p
          className={`text-2xl font-semibold tabular-nums ${tone === 'bad' ? 'text-destructive' : 'text-foreground'}`}
        >
          {value}
        </p>
        {note ? <p className="text-xs text-muted-foreground">{note}</p> : null}
      </CardContent>
    </Card>
  )
}

function SloCard({
  app,
  slo,
}: Readonly<{ app: string; slo: TrafficDetail['slo'] }>) {
  const { t } = useTranslation('trafficDetail')
  if (!slo) {
    return (
      <Card>
        <CardContent className="flex flex-wrap items-center justify-between gap-3 py-4">
          <p className="text-sm text-muted-foreground">{t('slo.none')}</p>
          <Link
            to="/apps/$name/alerts"
            params={{ name: app }}
            className="text-sm underline underline-offset-4"
          >
            {t('slo.noneAction')}
          </Link>
        </CardContent>
      </Card>
    )
  }
  const burning = slo.burn_rate >= 1
  return (
    <Card>
      <CardContent className="flex flex-wrap items-center gap-6 py-4">
        <div>
          <p className="text-sm font-medium text-foreground">
            {t('slo.title')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('slo.objective', {
              objective: slo.objective,
              target: slo.target,
            })}
          </p>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">{t('slo.burn')}</p>
          <Badge variant={burning ? 'destructive' : 'muted'}>
            {slo.burn_rate.toFixed(2)}x
          </Badge>
        </div>
        <div>
          <p className="text-xs text-muted-foreground">{t('slo.remaining')}</p>
          <p className="text-lg font-semibold tabular-nums">
            {(slo.budget_remaining * 100).toFixed(1)}%
          </p>
        </div>
        <p className="text-xs text-muted-foreground">{t('slo.burnHint')}</p>
      </CardContent>
    </Card>
  )
}

/** TrafficDetailPanel is the app Traffic tab: RED totals, live charts, replicas, slow routes and samples. */
export function TrafficDetailPanel({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('trafficDetail')
  const [win, setWin] = useState<TrafficWindow>('5m')
  const { data, error, isPending, refetch } = useTrafficDetail(app, win)

  const range = useMemo(
    () =>
      data ? { from: new Date(data.from), to: new Date(data.to) } : undefined,
    [data],
  )
  const { t: tObs } = useTranslation('observability')
  const attempts = useQuery(deployAttemptsQueryOptions(app))
  const [detail, setDetail] = useState<DeployAttempt | null>(null)
  const markers = useMemo<ChartMarker[]>(
    () =>
      range
        ? deployMarkers(attempts.data ?? [], range, (a, at) =>
            tObs('markers.deploy', {
              status: tObs(`deployStatus.${a.status}`),
              image: a.image,
              time: new Date(at).toLocaleString(),
            }),
          )
        : [],
    [attempts.data, range, tObs],
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-xl text-sm text-muted-foreground">
          {t('subtitle')}
        </p>
        <div
          role="group"
          aria-label={t('windowLabel')}
          className="flex items-center gap-1"
        >
          {WINDOWS.map((w) => (
            <Button
              key={w}
              size="sm"
              variant={w === win ? 'secondary' : 'outline'}
              aria-pressed={w === win}
              onClick={() => {
                setWin(w)
              }}
            >
              {t(`windows.${w}`)}
            </Button>
          ))}
        </div>
      </div>

      {error && !data ? (
        <Alert variant="destructive">
          <AlertTitle>{t('error.title')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{t('error.description', { message: error.message })}</p>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void refetch()
              }}
            >
              {t('error.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      {isPending ? <Skeleton className="h-28 w-full" /> : null}

      <AbuseDefenseCard app={app} counters={data?.abuse ?? NO_ABUSE} />

      {data && !data.has_traffic ? (
        <EmptyState
          icon={<ChartLineGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
          action={
            <Link
              to="/apps/$name/domains"
              params={{ name: app }}
              className="text-sm underline underline-offset-4"
            >
              {t('empty.action')}
            </Link>
          }
        />
      ) : null}

      {data?.has_traffic ? (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Kpi
              label={t('kpi.rate')}
              value={formatRate(data.rate_per_sec)}
              note={t('kpi.requests', { count: data.requests })}
            />
            <Kpi
              label={t('kpi.error5xx')}
              value={formatPercent(data.error_rate_5xx)}
              note={t('kpi.upstreamErrors', { count: data.upstream_errors })}
              tone={data.error_rate_5xx > 0 ? 'bad' : undefined}
            />
            <Kpi
              label={t('kpi.error4xx')}
              value={formatPercent(data.error_rate_4xx)}
            />
            <Kpi
              label={t('kpi.p99')}
              value={formatMs(data.latency.p99_ms)}
              note={`${t('kpi.p50')} ${formatMs(data.latency.p50_ms)}, ${t('kpi.p95')} ${formatMs(data.latency.p95_ms)}`}
            />
          </div>
          <SloCard app={app} slo={data.slo} />
          <ErrorSpikesCard spikes={data.error_spikes ?? []} />
          <ShieldBlockedCard app={app} shield={data.shield} />
          {range ? (
            <>
              {markers.length > 0 ? (
                <p className="text-xs text-muted-foreground">
                  {t('markers.hint')}
                </p>
              ) : null}
              <RequestMetricsSection
                appName={app}
                range={range}
                markers={markers}
                live
                onMarkerClick={(marker) => {
                  setDetail(
                    (attempts.data ?? []).find((a) => a.id === marker.key) ??
                      null,
                  )
                }}
              />
              <DeployMarkerDetails
                appName={app}
                attempt={detail}
                onClose={() => {
                  setDetail(null)
                }}
              />
            </>
          ) : null}
          <ReplicaTable rows={data.replicas} />
          <TopRoutesCard
            rows={data.top_routes ?? []}
            totals={data.kind_totals}
          />
          <SlowRouteTable rows={data.slow_routes} />
          <div className="grid gap-4 lg:grid-cols-2">
            <AudienceTable kind="referrers" rows={data.referrers} />
            <AudienceTable kind="countries" rows={data.countries} />
          </div>
          <SampleList rows={data.samples} />
        </>
      ) : null}
    </div>
  )
}
