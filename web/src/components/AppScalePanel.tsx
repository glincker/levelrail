import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { autoscaleQueryOptions } from '../queries/autoscale'
import {
  useAppLoadBalancer,
  useAppLoadBalancerStatus,
  type UpstreamState,
} from '../queries/appLoadBalancer'
import { useAppMetricsRow } from '../queries/appsMetrics'
import { formatBytes, formatSize } from '../lib/format'
import type { AppDetail } from '../types/appDetail'

const STATES: readonly UpstreamState[] = [
  'healthy',
  'unhealthy',
  'draining',
  'disabled',
  'unknown',
]

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </dt>
      <dd className="mt-0.5 text-sm text-foreground">{value}</dd>
    </div>
  )
}

// Read-only "after you deploy" view: live numbers only, no sizing tables.
export function AppScalePanel({ app }: { app: AppDetail }) {
  const { t } = useTranslation('resources', { keyPrefix: 'scale' })
  const { t: tc } = useTranslation('common')
  const metrics = useAppMetricsRow(app.name)
  const autoscale = useQuery(autoscaleQueryOptions(app.name))
  const lb = useAppLoadBalancer(app.name)
  const configured = lb.data?.configured === true
  const status = useAppLoadBalancerStatus(app.name, configured)

  const m = metrics.data
  const policy = autoscale.data
  const limit = app.resources?.memory_bytes
  const upstreams = configured ? (status.data?.upstreams ?? []) : []

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{t('title')}</CardTitle>
          <Badge variant="outline">{t('tag')}</Badge>
        </div>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-5">
        <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <Stat label={t('replicas')} value={String(app.replicas)} />
          <Stat
            label={t('memoryLimit')}
            value={limit ? formatBytes(limit) : t('noLimit')}
          />
          <Stat
            label={t('cpu')}
            value={
              m?.cpu_percent === undefined
                ? t('unknown')
                : `${m.cpu_percent.toFixed(0)}%`
            }
          />
          <Stat
            label={t('memory')}
            value={
              m?.memory_usage_bytes === undefined
                ? t('unknown')
                : formatSize(m.memory_usage_bytes)
            }
          />
        </dl>

        {app.replicas > 1 ? (
          <Alert role="status">
            <AlertDescription>
              {tc('replicaNotice.sameServer', { replicas: app.replicas })}
            </AlertDescription>
          </Alert>
        ) : null}

        <div className="space-y-1">
          <h3 className="text-sm font-medium">{t('autoscale')}</h3>
          <p className="text-sm text-muted-foreground">
            {policy
              ? t('autoscaleOn', {
                  min: policy.min_replicas,
                  max: policy.max_replicas,
                })
              : t('autoscaleOff')}
          </p>
          {policy ? (
            <p className="text-sm text-muted-foreground">
              {policy.evaluated_at
                ? t('lastDecision', {
                    reason: policy.condition.message || policy.condition.reason,
                    at: new Date(policy.evaluated_at).toLocaleTimeString(),
                  })
                : t('noDecision')}
            </p>
          ) : null}
        </div>

        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t('replicaHealth')}</h3>
          {upstreams.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('noReplicaHealth')}
            </p>
          ) : (
            <ul className="divide-y rounded-md border">
              {upstreams.map((u) => (
                <li
                  key={u.id}
                  className="flex items-center justify-between gap-3 px-3 py-2 text-sm"
                >
                  <span>{t('replicaRow', { n: u.replica + 1 })}</span>
                  <Badge variant={u.healthy ? 'success' : 'warning'}>
                    {t(
                      `state.${STATES.includes(u.state) ? u.state : 'unknown'}`,
                    )}
                  </Badge>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t('guide.title')}</h3>
          <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            <li>{t('guide.resize')}</li>
            <li>{t('guide.replicas')}</li>
            <li>{t('guide.autoscale')}</li>
            <li>{t('guide.biggerHost')}</li>
          </ul>
        </div>
      </CardContent>
    </Card>
  )
}
