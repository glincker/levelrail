import { lazy, Suspense, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CheckCircleIcon,
  GaugeIcon,
  StackIcon,
  WarningCircleIcon,
  WrenchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { PageBanner } from '@/components/PageBanner'
import { LazyHealthDiagram } from '@/components/banners/LazyInfraDiagrams'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { IntegrityCard } from '@/components/settings/IntegrityCard'
import { ReconcileGuardCard } from '@/components/settings/ReconcileGuardCard'
import {
  useApplyControlPlaneFix,
  useControlPlaneHealth,
  type CPHealth,
  type CPHealthFinding,
  type CPHealthFix,
} from '../../queries/controlPlaneHealth'

const FootprintCard = lazy(() =>
  import('@/components/settings/FootprintCard').then((m) => ({
    default: m.FootprintCard,
  })),
)

const MIB = 1 << 20

function formatFinding(f: CPHealthFinding): { value: string; limit: string } {
  switch (f.id) {
    case 'heap':
    case 'wal_size':
    case 'rss':
    case 'telemetry_size':
      return {
        value: `${Math.round(f.value / MIB)} MiB`,
        limit: `${Math.round(f.threshold / MIB)} MiB`,
      }
    case 'db_latency':
      return {
        value: `${f.value.toFixed(1)} ms`,
        limit: `${f.threshold.toFixed(0)} ms`,
      }
    case 'reconcile_pass':
    case 'event_lag':
      return {
        value: `${f.value.toFixed(0)} s`,
        limit: `${f.threshold.toFixed(0)} s`,
      }
    default:
      return { value: f.value.toFixed(0), limit: f.threshold.toFixed(0) }
  }
}

function Metric({
  label,
  value,
  hint,
  icon,
}: {
  label: string
  value: string
  hint?: string
  icon?: ReactNode
}) {
  return (
    <div className="space-y-1">
      <p className="flex items-center gap-1 text-xs text-muted-foreground">
        {icon}
        {label}
      </p>
      <p className="text-lg font-medium tabular-nums">{value}</p>
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

function MetricsGrid({ h }: { h: CPHealth }) {
  const { t } = useTranslation('settings', {
    keyPrefix: 'controlPlaneHealth.metrics',
  })
  return (
    <div className="grid grid-cols-2 gap-6 md:grid-cols-4">
      <Metric label={t('goroutines')} value={String(h.runtime.goroutines)} />
      <Metric
        label={t('heap')}
        value={`${Math.round(h.runtime.heap_bytes / MIB)} MiB`}
      />
      <Metric
        label={t('dbLatency')}
        value={`${h.db.p95_ms.toFixed(1)} ms`}
        hint={t('dbLatencyHint', {
          p50: h.db.p50_ms.toFixed(1),
          p99: h.db.p99_ms.toFixed(1),
        })}
      />
      <Metric
        label={t('dbQueue')}
        value={String(h.db.wait_count)}
        hint={t('dbQueueHint', { ms: Math.round(h.db.wait_ms) })}
      />
      <Metric
        label={t('readQueue')}
        value={h.db.read_pool_enabled ? String(h.db.read_wait_count) : '-'}
        hint={
          h.db.read_pool_enabled
            ? t('readQueueHint', {
                ms: Math.round(h.db.read_wait_ms),
                inUse: h.db.read_in_use,
                size: h.db.read_pool_size,
              })
            : t('readPoolDisabled')
        }
        icon={<StackIcon aria-hidden className="size-3.5" />}
      />
      <Metric
        label={t('wal')}
        value={`${Math.round(h.db.wal_bytes / MIB)} MiB`}
        hint={t('walHint', { size: Math.round(h.db.db_bytes / MIB) })}
      />
      <Metric
        label={t('reconcile')}
        value={`${h.reconcile.last_pass_ms.toFixed(0)} ms`}
        hint={t('reconcileHint', {
          n: h.reconcile.controllers,
          p95: h.reconcile.p95_ms.toFixed(0),
        })}
      />
      {h.reconcile.queue && (
        <Metric
          label={t('reconcileQueue')}
          value={t('reconcileQueueValue', {
            inFlight: h.reconcile.queue.in_flight,
            workers: h.reconcile.queue.workers,
          })}
          hint={t('reconcileQueueHint', {
            depth: h.reconcile.queue.queue_depth,
            wait: h.reconcile.queue.oldest_wait_seconds.toFixed(1),
          })}
        />
      )}
      <Metric
        label={t('eventLag')}
        value={`${h.reconcile.lag_seconds.toFixed(0)} s`}
      />
      <Metric
        label={t('streams')}
        value={`${h.streams.active} / ${h.streams.cap}`}
      />
      <Metric
        label={t('rss')}
        value={`${Math.round(h.footprint.rss_bytes / MIB)} MiB`}
        hint={
          h.footprint.memory_limit.limit_bytes > 0
            ? t('rssHint', {
                limit: Math.round(h.footprint.memory_limit.limit_bytes / MIB),
              })
            : t('rssNoLimit')
        }
      />
      <Metric label={t('threads')} value={String(h.footprint.threads)} />
      <Metric
        label={t('cpu')}
        value={`${h.footprint.cpu_percent.toFixed(1)} %`}
      />
      <Metric
        label={t('telemetry')}
        value={`${Math.round(h.footprint.telemetry_bytes / MIB)} MiB`}
      />
    </div>
  )
}

export function ControlPlaneHealthPage() {
  const { t } = useTranslation('settings')
  const { data, isPending, error, refetch, isFetching } =
    useControlPlaneHealth()
  const fix = useApplyControlPlaneFix()

  const apply = (id: CPHealthFix) => {
    fix.mutate(
      { fix: id },
      {
        onSuccess: () => {
          toast.add({
            title: t('controlPlaneHealth.fixApplied'),
            type: 'success',
          })
        },
        onError: (e) => {
          toast.add({
            title: t('controlPlaneHealth.fixFailed'),
            description: e.message,
            type: 'error',
          })
        },
      },
    )
  }

  const hero = Boolean(error) || (data?.findings.length ?? 0) > 0
  const size = hero ? 'hero' : 'compact'
  const statusVariant =
    data?.status === 'ok'
      ? 'success'
      : data?.status === 'warn'
        ? 'warning'
        : 'destructive'

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<GaugeIcon />}
        titleKey="settings:controlPlaneHealth.title"
        descriptionKey={
          hero
            ? 'settings:controlPlaneHealth.banner.description'
            : 'settings:controlPlaneHealth.description'
        }
        action={
          <>
            {data ? (
              <Badge variant={statusVariant}>
                {data.status === 'ok' ? (
                  <CheckCircleIcon aria-hidden="true" />
                ) : (
                  <WarningCircleIcon aria-hidden="true" />
                )}
                {t(`controlPlaneHealth.status.${data.status}`)}
              </Badge>
            ) : null}
            <Button
              type="button"
              size="sm"
              disabled={isFetching}
              onClick={() => {
                void refetch()
              }}
            >
              <ArrowsClockwiseIcon aria-hidden="true" />
              {isFetching
                ? t('controlPlaneHealth.rechecking')
                : t('controlPlaneHealth.recheck')}
            </Button>
          </>
        }
        illustration={<LazyHealthDiagram size={size} />}
        size={size}
      />
      {isPending ? <Skeleton className="h-48 w-full" /> : null}
      {error ? (
        <Banner
          variant="critical"
          title={t('controlPlaneHealth.errorTitle')}
          action={
            <Button
              type="button"
              size="sm"
              onClick={() => {
                void refetch()
              }}
            >
              {t('controlPlaneHealth.retry')}
            </Button>
          }
        >
          {error.message}
        </Banner>
      ) : null}
      {data ? (
        <div className="space-y-(--glinr-page-gap)">
          {data.findings.length === 0 ? (
            <Banner
              variant="success"
              title={t('controlPlaneHealth.allClear')}
            />
          ) : (
            <section aria-labelledby="cp-findings" className="space-y-3">
              <h2 id="cp-findings" className="text-sm font-semibold">
                {t('controlPlaneHealth.findingsTitle')}{' '}
                <span className="font-normal text-muted-foreground">
                  {t('controlPlaneHealth.findingsCount', {
                    count: data.findings.length,
                  })}
                </span>
              </h2>
              {data.findings.map((f) => {
                const v = formatFinding(f)
                return (
                  <Card key={f.id}>
                    <CardContent className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 space-y-2">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">
                            {t(`controlPlaneHealth.findings.${f.id}.title`)}
                          </span>
                          <Badge
                            variant={
                              f.severity === 'crit' ? 'destructive' : 'warning'
                            }
                          >
                            {t(`controlPlaneHealth.severity.${f.severity}`)}
                          </Badge>
                          <Badge variant="outline">
                            {t('controlPlaneHealth.readingChip', {
                              value: v.value,
                              limit: v.limit,
                            })}
                          </Badge>
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {t(`controlPlaneHealth.findings.${f.id}.hint`)}
                        </p>
                      </div>
                      {f.fix ? (
                        <div className="space-y-2 sm:text-right">
                          <Button
                            size="sm"
                            disabled={fix.isPending}
                            onClick={() => {
                              if (f.fix) apply(f.fix)
                            }}
                          >
                            <WrenchIcon />
                            {t(`controlPlaneHealth.fixes.${f.fix}`)}
                          </Button>
                          <Collapsible>
                            <CollapsibleTrigger className="text-xs text-muted-foreground underline">
                              {t('controlPlaneHealth.fixViaCli')}
                            </CollapsibleTrigger>
                            <CollapsiblePanel>
                              <code className="mt-1 block text-xs break-all">
                                {`system health --fix ${f.fix}`}
                              </code>
                            </CollapsiblePanel>
                          </Collapsible>
                        </div>
                      ) : null}
                    </CardContent>
                  </Card>
                )
              })}
            </section>
          )}
          {data.idle_footprint ? (
            <Suspense fallback={<Skeleton className="h-40 w-full" />}>
              <FootprintCard idle={data.idle_footprint} />
            </Suspense>
          ) : null}
          <IntegrityCard />
          <ReconcileGuardCard guard={data.reconcile.guard} />
          <Card>
            <CardHeader>
              <CardTitle>{t('controlPlaneHealth.metricsTitle')}</CardTitle>
            </CardHeader>
            <CardContent>
              <MetricsGrid h={data} />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('controlPlaneHealth.loops.title')}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {data.footprint.loops.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('controlPlaneHealth.loops.empty')}
                </p>
              ) : (
                data.footprint.loops.map((l) => (
                  <div
                    key={l.name}
                    className="flex flex-wrap items-center justify-between gap-3 text-sm"
                  >
                    <span className="font-medium">{l.name}</span>
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {t('controlPlaneHealth.loops.row', {
                        every: Math.round(l.interval_seconds),
                        runs: l.runs,
                        ms: l.last_duration_ms.toFixed(1),
                        cost: l.cost_percent.toFixed(3),
                      })}
                    </span>
                    {l.last_error ? (
                      <Badge variant="warning">{l.last_error}</Badge>
                    ) : null}
                  </div>
                ))
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>
                {t('controlPlaneHealth.slowQueries', {
                  count: data.db.slow_queries,
                  ms: data.db.slow_threshold_ms,
                })}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {data.db.recent_slow.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('controlPlaneHealth.noSlowQueries')}
                </p>
              ) : (
                data.db.recent_slow
                  .slice(-10)
                  .reverse()
                  .map((q) => (
                    <div
                      key={`${q.at}-${q.sql}`}
                      className="flex items-start justify-between gap-3 text-xs"
                    >
                      <code className="min-w-0 break-all">{q.sql}</code>
                      <span className="shrink-0 tabular-nums">
                        {q.duration_ms.toFixed(0)} ms
                      </span>
                    </div>
                  ))
              )}
            </CardContent>
          </Card>
        </div>
      ) : null}
    </div>
  )
}
