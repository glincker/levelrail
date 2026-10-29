import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { PlayGlassIcon, ShieldCheckGlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { formatAge } from '../lib/format'
import {
  useControlPlaneDr,
  useRunControlPlaneDrBackup,
  useRunControlPlaneDrDrill,
  type ControlPlaneDrDrill,
  type ControlPlaneDrReplica,
} from '../queries/controlPlaneDr'
import { ControlPlaneDrChecklistView } from './ControlPlaneDrChecklist'
import { ControlPlaneDrEscrow } from './ControlPlaneDrEscrow'
import { ControlPlaneDrRestore } from './ControlPlaneDrRestore'
import { ControlPlaneDrSettings } from './ControlPlaneDrSettings'

function drillSummary(
  d: ControlPlaneDrDrill,
  t: TFunction<'settings'>,
): string {
  if (d.at === undefined) return t('drCard.neverRun')
  const when = formatAge(d.at)
  if (!d.ok) return t('drCard.failed', { when, detail: d.detail })
  if (d.partial) return t('drCard.passedPartial', { when })
  return t('drCard.passed', {
    when,
    seconds: Math.max(1, Math.round(d.duration_ms / 1000)),
  })
}

function replicaSummary(
  r: ControlPlaneDrReplica,
  t: TFunction<'controlPlaneReplica'>,
): string {
  const parts: string[] = []
  parts.push(
    r.last_ship_at === undefined
      ? t('neverShipped', { seconds: r.interval_seconds })
      : t('summary', {
          seconds: r.interval_seconds,
          age: formatAge(r.last_ship_at),
          kb: Math.round((r.last_size_bytes ?? 0) / 1024),
          ms: r.last_ship_ms ?? 0,
        }),
  )
  if (r.pending_since !== undefined) {
    parts.push(t('pending', { age: formatAge(r.pending_since) }))
  }
  if (r.last_error) parts.push(t('error', { message: r.last_error }))
  return parts.join('; ')
}

export function ControlPlaneDrCard() {
  const { t } = useTranslation('settings')
  const { t: tr } = useTranslation('controlPlaneReplica')
  const { data: status, isLoading, isError } = useControlPlaneDr()
  const run = useRunControlPlaneDrBackup()
  const drill = useRunControlPlaneDrDrill()

  // Returns 501 when there is no master key and 403 for a non-root session.
  if (isLoading || isError || !status) {
    return null
  }

  const start = (mutate: typeof run.mutate, label: string) => {
    mutate(undefined, {
      onSuccess: () => {
        toast.add({
          title: t('drCard.started', { label }),
          type: 'success',
        })
      },
      onError: (e) => {
        toast.add({ title: e.message, type: 'error' })
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <ShieldCheckGlassIcon className="size-4.5 text-tone-success" />
            </div>
            <div>
              <CardTitle>{t('drCard.title')}</CardTitle>
              <CardDescription>{t('drCard.description')}</CardDescription>
            </div>
          </div>
          <div className="flex gap-2">
            <Button
              size="sm"
              disabled={
                run.isPending || status.backup_running || !status.configured
              }
              onClick={() => {
                start(run.mutate, t('drCard.labelBackup'))
              }}
            >
              <PlayGlassIcon className="size-4" aria-hidden="true" />
              {status.backup_running
                ? t('drCard.backingUp')
                : t('drCard.runNow')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={
                drill.isPending || status.drill_running || !status.configured
              }
              onClick={() => {
                start(drill.mutate, t('drCard.labelDrill'))
              }}
            >
              {status.drill_running
                ? t('drCard.runningDrill')
                : t('drCard.runDrill')}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-5">
        {status.warnings.length > 0 ? (
          <div className="space-y-2" aria-label={t('drCard.warnings')}>
            {status.warnings.map((w) => (
              <Banner key={w.code} variant="warning" title={w.message} />
            ))}
          </div>
        ) : null}

        <ControlPlaneDrChecklistView checklist={status.checklist} />

        <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
          <dt className="text-muted-foreground">{t('drCard.lastBackup')}</dt>
          <dd>
            {status.last_backup_at !== undefined
              ? formatAge(status.last_backup_at)
              : t('drCard.never')}
            {status.next_backup_at !== undefined
              ? `, ${t('drCard.nextBackup', { when: new Date(status.next_backup_at).toLocaleString() })}`
              : ''}
          </dd>
          {status.replica ? (
            <>
              <dt className="text-muted-foreground">{tr('label')}</dt>
              <dd>{replicaSummary(status.replica, tr)}</dd>
            </>
          ) : null}
          <dt className="text-muted-foreground">{t('drCard.lastDrill')}</dt>
          <dd>{drillSummary(status.last_drill, t)}</dd>
          <dt className="text-muted-foreground">{t('drCard.drillIdentity')}</dt>
          <dd>
            {status.drill_identity_configured
              ? t('drCard.identityConfigured')
              : t('drCard.identityMissing')}
          </dd>
        </dl>

        <ControlPlaneDrEscrow status={status} />

        <ControlPlaneDrRestore configured={status.configured} />

        <div className="border-t border-border pt-4">
          <ControlPlaneDrSettings
            key={`${status.target_id}|${status.recipients.join(',')}|${status.enabled}`}
            status={status}
          />
        </div>
      </CardContent>
    </Card>
  )
}
