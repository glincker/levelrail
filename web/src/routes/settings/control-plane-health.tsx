import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { GaugeIcon, WrenchIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
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
import {
  useApplyControlPlaneFix,
  useControlPlaneHealth,
  type CPHealth,
  type CPHealthFinding,
  type CPHealthFix,
} from '../../queries/controlPlaneHealth'

export const Route = createFileRoute('/settings/control-plane-health')({
  component: ControlPlaneHealthPage,
})

const MIB = 1 << 20

function formatFinding(f: CPHealthFinding): { value: string; limit: string } {
  switch (f.id) {
    case 'heap':
    case 'wal_size':
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
}: {
  label: string
  value: string
  hint?: string
}) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{label}</p>
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
      <Metric
        label={t('eventLag')}
        value={`${h.reconcile.lag_seconds.toFixed(0)} s`}
      />
      <Metric
        label={t('streams')}
        value={`${h.streams.active} / ${h.streams.cap}`}
      />
    </div>
  )
}

function ControlPlaneHealthPage() {
  const { t } = useTranslation('settings')
  const { data, isPending, error } = useControlPlaneHealth()
  const fix = useApplyControlPlaneFix()

  const apply = (id: CPHealthFix) => {
    fix.mutate(id, {
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
    })
  }

  return (
    <PageTemplate
      icon={<GaugeIcon />}
      title={t('controlPlaneHealth.title')}
      description={t('controlPlaneHealth.description')}
      status={
        data ? (
          <Badge
            variant={
              data.status === 'ok'
                ? 'success'
                : data.status === 'warn'
                  ? 'warning'
                  : 'destructive'
            }
          >
            {t(`controlPlaneHealth.status.${data.status}`)}
          </Badge>
        ) : null
      }
    >
      {isPending ? <Skeleton className="h-48 w-full" /> : null}
      {error ? <Banner variant="critical" title={error.message} /> : null}
      {data ? (
        <div className="space-y-6">
          {data.findings.length === 0 ? (
            <Banner
              variant="success"
              title={t('controlPlaneHealth.allClear')}
            />
          ) : (
            <div className="space-y-3">
              {data.findings.map((f) => {
                const v = formatFinding(f)
                return (
                  <Card key={f.id}>
                    <CardContent className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 space-y-1">
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
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {t('controlPlaneHealth.reading', {
                            value: v.value,
                            limit: v.limit,
                          })}
                        </p>
                        <p className="text-sm text-muted-foreground">
                          {t(`controlPlaneHealth.findings.${f.id}.hint`)}
                        </p>
                      </div>
                      {f.fix ? (
                        <div className="space-y-2 text-right">
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
                              <code className="mt-1 block text-xs">
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
            </div>
          )}
          <Card>
            <CardContent>
              <MetricsGrid h={data} />
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
    </PageTemplate>
  )
}
