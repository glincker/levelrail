import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  autoscaleQueryOptions,
  useAutoscaleActions,
  type Autoscale,
} from '../queries/autoscale'
import { AutoscaleForm } from './autoscale/AutoscaleForm'
import { AutoscaleHistory } from './autoscale/AutoscaleHistory'

const REASONS = [
  'Pending',
  'WithinTarget',
  'ScaledUp',
  'ScaledDown',
  'MetricsUnavailable',
  'CooldownActive',
  'Stabilizing',
  'NoCapacity',
  'SpendCapPaused',
  'AtMaximum',
  'AtMinimum',
  'BoundsCorrected',
  'Suspended',
  'ScaleFailed',
  'Flapping',
] as const
type Reason = (typeof REASONS)[number]

function isReason(r: string): r is Reason {
  return (REASONS as readonly string[]).includes(r)
}

const NEXT_STEPS = [
  'no_capacity',
  'spend_cap',
  'no_metrics',
  'max_replicas',
  'global_ceiling',
] as const
type NextStep = (typeof NEXT_STEPS)[number]

function nextStep(by: string, flapping: boolean): NextStep | 'flapping' | null {
  if ((NEXT_STEPS as readonly string[]).includes(by)) return by as NextStep
  return flapping ? 'flapping' : null
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium tabular-nums">{value}</dd>
    </div>
  )
}

function Summary({ policy }: { policy: Autoscale }) {
  const { t } = useTranslation('autoscale')
  const unit =
    policy.metric === 'requests_per_replica' ? t('form.perSecond') : '%'
  const metric = policy.value_known
    ? t('stats.metricValue', {
        value: policy.value.toFixed(1),
        target: policy.target,
        unit,
      })
    : t('stats.metricUnknown')
  return (
    <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Stat
        label={t('stats.replicas')}
        value={t('stats.replicasValue', {
          current: policy.current,
          desired: policy.desired,
        })}
      />
      <Stat
        label={t('stats.range')}
        value={t('stats.rangeValue', {
          min: policy.min_replicas,
          max: policy.effective_max,
        })}
      />
      <Stat label={t(`metric.${policy.metric}`)} value={metric} />
      <Stat
        label={t('stats.band')}
        value={t('stats.bandValue', {
          up: policy.scale_up_tolerance_pct,
          down: policy.scale_down_tolerance_pct,
        })}
      />
    </dl>
  )
}

/** AppAutoscaleCard shows an app's replica autoscaler, its latest decision and history, and lets the operator set, change or remove the policy. */
export function AppAutoscaleCard({ appName }: { appName: string }) {
  const { t } = useTranslation('autoscale')
  const query = useQuery(autoscaleQueryOptions(appName))
  const { save, clear } = useAutoscaleActions(appName)
  const [editing, setEditing] = useState(false)
  const policy = query.data ?? null

  let body
  if (query.isPending) {
    body = <Skeleton className="h-16 w-full" aria-label={t('loading')} />
  } else if (query.isError) {
    body = (
      <Alert variant="destructive" role="alert">
        <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
          <span>{t('loadError')}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            {t('retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (editing || (!policy && save.isPending)) {
    body = (
      <AutoscaleForm
        policy={policy}
        pending={save.isPending}
        serverError={save.error?.message ?? null}
        onCancel={() => {
          save.reset()
          setEditing(false)
        }}
        onSave={(fields) =>
          save.mutate(fields, {
            onSuccess: () => {
              setEditing(false)
              toast.add({ title: t('toast.saved'), type: 'success' })
            },
          })
        }
      />
    )
  } else if (!policy) {
    body = (
      <EmptyState
        compact
        icon={<LayersGlassIcon className="size-6" />}
        title={t('empty.title')}
        description={t('empty.body')}
        action={
          <Button type="button" size="sm" onClick={() => setEditing(true)}>
            {t('empty.action')}
          </Button>
        }
      />
    )
  } else {
    const reason = policy.condition.reason
    const step = nextStep(policy.blocked_by, policy.flapping)
    body = (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Badge variant={policy.condition.healthy ? 'success' : 'warning'}>
            {isReason(reason) ? t(`reason.${reason}`) : reason}
          </Badge>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setEditing(true)}
            >
              {t('actions.edit')}
            </Button>
            <ConfirmAction
              trigger={(open) => (
                <Button type="button" size="sm" variant="ghost" onClick={open}>
                  {t('actions.remove')}
                </Button>
              )}
              title={t('remove.title')}
              description={t('remove.description')}
              consequences={[
                t('remove.consequences.replicas'),
                t('remove.consequences.manual'),
                t('remove.consequences.history'),
              ]}
              confirmLabel={t('remove.confirm')}
              pendingLabel={t('remove.pending')}
              pending={clear.isPending}
              error={clear.error?.message ?? null}
              onOpenChange={() => clear.reset()}
              onConfirm={(close) =>
                clear.mutate(undefined, {
                  onSuccess: () => {
                    close()
                    toast.add({ title: t('toast.removed'), type: 'success' })
                  },
                })
              }
            />
          </div>
        </div>
        <p className="text-sm text-muted-foreground">
          {policy.condition.message}
        </p>
        {step ? (
          <Alert role="status">
            <AlertDescription>{t(`next.${step}`)}</AlertDescription>
          </Alert>
        ) : null}
        <Summary policy={policy} />
        <AutoscaleHistory appName={appName} />
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
