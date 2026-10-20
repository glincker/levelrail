import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChartLineUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { EmptyState } from '@/components/ui/empty-state'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '../lib/format'
import {
  spendCapQueryOptions,
  useSpendCapActions,
  type SpendCap,
} from '../queries/spendCap'
import { BudgetForm } from './budget/BudgetForm'
import { BudgetHistory } from './budget/BudgetHistory'

const REASONS = [
  'WithinBudget',
  'ThresholdReached',
  'LimitReachedAlertOnly',
  'Throttled',
  'Paused',
  'EnforcementPending',
] as const
type Reason = (typeof REASONS)[number]

function isReason(r: string): r is Reason {
  return (REASONS as readonly string[]).includes(r)
}

function UsageBar({
  label,
  detail,
  pct,
}: {
  label: string
  detail: string
  pct: number
}) {
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">{detail}</p>
      <Progress value={Math.min(Math.max(pct, 0), 100)}>
        <ProgressLabel>{label}</ProgressLabel>
        <ProgressValue>{() => `${Math.round(pct)}%`}</ProgressValue>
      </Progress>
    </div>
  )
}

function Usage({ cap }: { cap: SpendCap }) {
  const { t } = useTranslation('budget')
  return (
    <div className="space-y-3">
      {cap.monthly_egress_bytes > 0 ? (
        <UsageBar
          label={t('usage.egress')}
          detail={t('usage.ofLimit', {
            used: formatBytes(cap.egress_bytes) || '0 B',
            limit: formatBytes(cap.monthly_egress_bytes),
          })}
          pct={cap.egress_pct}
        />
      ) : null}
      {cap.monthly_cost_ceiling > 0 ? (
        <>
          <UsageBar
            label={t('usage.cost')}
            detail={t('usage.costOfLimit', {
              used: cap.cost.toFixed(2),
              limit: cap.monthly_cost_ceiling.toFixed(2),
            })}
            pct={cap.cost_pct}
          />
          {cap.cost_priced ? null : (
            <p className="text-xs text-muted-foreground">
              {t('usage.notPriced')}
            </p>
          )}
        </>
      ) : null}
    </div>
  )
}

function Summary({ cap }: { cap: SpendCap }) {
  const { t } = useTranslation('budget')
  return (
    <ul className="space-y-0.5 text-xs text-muted-foreground">
      <li>
        {t('summary.alerts', {
          levels: cap.thresholds.map((p) => `${p}%`).join(', '),
        })}
      </li>
      <li>{t(`summary.action.${cap.action}`, { rps: cap.throttle_rps })}</li>
    </ul>
  )
}

/** AppBudgetCard shows an app's monthly egress and cost cap, and lets the operator set, change or remove it. */
export function AppBudgetCard({ appName }: { appName: string }) {
  const { t } = useTranslation('budget')
  const query = useQuery(spendCapQueryOptions(appName))
  const { save, clear } = useSpendCapActions(appName)
  const [editing, setEditing] = useState(false)
  const cap = query.data ?? null

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
  } else if (editing || (!cap && save.isPending)) {
    body = (
      <BudgetForm
        cap={cap}
        pending={save.isPending}
        serverError={save.error?.message ?? null}
        onCancel={() => {
          save.reset()
          setEditing(false)
        }}
        onSave={(update) =>
          save.mutate(update, {
            onSuccess: () => {
              setEditing(false)
              toast.add({ title: t('toast.saved'), type: 'success' })
            },
          })
        }
      />
    )
  } else if (!cap) {
    body = (
      <EmptyState
        compact
        icon={<ChartLineUpIcon className="size-5" />}
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
    const reason = cap.condition.reason
    body = (
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Badge variant={cap.condition.healthy ? 'success' : 'warning'}>
              {isReason(reason) ? t(`reason.${reason}`) : reason}
            </Badge>
            <span className="text-xs text-muted-foreground">
              {t('status.resets', {
                date: new Date(cap.resets_at).toLocaleDateString(),
              })}
            </span>
          </div>
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
                t('remove.consequences.lift'),
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
        <p className="text-sm text-muted-foreground">{cap.condition.message}</p>
        <Usage cap={cap} />
        <Summary cap={cap} />
        <BudgetHistory appName={appName} />
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
