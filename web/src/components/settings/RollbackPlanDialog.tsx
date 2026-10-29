import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { StatusPill, type Tone } from '@/components/kit'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '../ui/dialog'
import { Button } from '../ui/button'
import { Alert, AlertDescription, AlertTitle } from '../ui/alert'
import {
  rollbackPlanQueryOptions,
  useStartRollback,
  type RollbackPlan,
} from '../../queries/releases'
import { ConfirmDialog } from '../ui/confirm-dialog'
import { toast } from '../ui/toast'
import type { UpgradeCheckStatus } from '../../queries/updates'
import { FixCli, type CliStep } from '../fixit/FixCli'
import { ReleaseNotes } from './ReleaseNotes'

const STATUS_TONE: Record<UpgradeCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
  pending: 'info',
  skipped: 'neutral',
}

const STEP_KEYS = {
  backup: 'plan.step.backup',
  fetch: 'plan.step.fetch',
  verify: 'plan.step.verify',
  stop: 'plan.step.stop',
  restore_backup: 'plan.step.restore_backup',
  swap_binary: 'plan.step.swap_binary',
  start: 'plan.step.start',
  verify_health: 'plan.step.verify_health',
  auto_recover: 'plan.step.auto_recover',
} as const

function isStep(s: string): s is keyof typeof STEP_KEYS {
  return s in STEP_KEYS
}

function schemaLabel(v: number): string | null {
  return v >= 0 ? String(v) : null
}

function when(iso: string): string {
  return new Date(iso).toLocaleString()
}

function DataLossBlock({ plan }: { plan: RollbackPlan }) {
  const { t } = useTranslation('updates')
  return (
    <Alert variant="destructive">
      <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
      <AlertTitle>{t('plan.dataLoss.title')}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{t('plan.dataLoss.body')}</p>
        {plan.data_loss_since ? (
          <p className="font-medium">
            {t('plan.dataLoss.since', { time: when(plan.data_loss_since) })}
          </p>
        ) : (
          <p className="font-medium">
            {t('plan.dataLoss.noneCompatible', {
              version: plan.target.version,
            })}
          </p>
        )}
        {plan.backups.length > 0 ? (
          <div className="space-y-1">
            <p className="text-xs font-medium">
              {t('plan.dataLoss.backupsTitle')}
            </p>
            <ul className="space-y-1 text-xs">
              {plan.backups.map((b) => (
                <li key={b.name} className="flex flex-wrap items-center gap-2">
                  <span className="font-mono">
                    {t('plan.dataLoss.backupRow', {
                      name: b.name,
                      time: when(b.created_at),
                      schema: b.schema_version,
                    })}
                  </span>
                  <StatusPill
                    tone={b.compatible ? 'success' : 'neutral'}
                    size="sm"
                    label={
                      b.compatible
                        ? t('plan.dataLoss.compatible')
                        : t('plan.dataLoss.incompatible')
                    }
                  />
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}

function rollbackSteps(plan: RollbackPlan, t: TFunction<'updates'>): CliStep[] {
  const steps: CliStep[] = []
  if (plan.fetch_command) {
    steps.push({
      label: t('plan.commands.fetch'),
      command: plan.fetch_command,
    })
  }
  if (plan.restore_required) {
    if (plan.restore_command) {
      steps.push({
        label: t('plan.commands.restore'),
        command: plan.restore_command,
      })
    }
  } else {
    steps.push({
      label: t('plan.commands.apply', { version: plan.target.version }),
      command: plan.command,
    })
  }
  return steps
}

function PlanBody({ plan }: { plan: RollbackPlan }) {
  const { t } = useTranslation('updates')
  const targetSchema = schemaLabel(plan.target.schema_version)
  return (
    <div className="space-y-4 text-sm">
      <div className="space-y-1">
        <p className="text-foreground">
          {t('plan.summary', {
            from: plan.current_version,
            schema: schemaLabel(plan.current_schema_version) ?? '?',
          })}{' '}
          {targetSchema
            ? t('plan.target', {
                version: plan.target.version,
                schema: targetSchema,
              })
            : t('plan.targetUnknown', { version: plan.target.version })}
        </p>
        <p className="text-muted-foreground">
          {t(`plan.verdict.${plan.verdict}`)}
        </p>
      </div>

      {plan.restore_required ? <DataLossBlock plan={plan} /> : null}

      {plan.blocked ? (
        <p role="alert" className="text-destructive">
          {t('plan.blocked')}
        </p>
      ) : null}

      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">
          {t('plan.stepsTitle')}
        </p>
        <ol className="ml-4 list-decimal space-y-0.5">
          {plan.steps.filter(isStep).map((s) => (
            <li key={s}>{t(STEP_KEYS[s], { version: plan.target.version })}</li>
          ))}
        </ol>
        <p className="text-xs text-muted-foreground">
          {t('plan.downtime', { seconds: plan.downtime_seconds })}
        </p>
      </div>

      <div className="space-y-1.5">
        <p className="text-xs font-medium text-muted-foreground">
          {t('plan.checksTitle')}
        </p>
        <ul className="space-y-2">
          {plan.checks.map((c) => (
            <li key={c.code} className="flex items-start gap-3">
              <StatusPill
                tone={STATUS_TONE[c.status]}
                label={t(`preflight.status.${c.status}`)}
                size="sm"
              />
              <span>
                <span className="font-medium text-foreground">{c.name}</span>
                <span className="block text-xs text-muted-foreground">
                  {c.message}
                </span>
              </span>
            </li>
          ))}
        </ul>
      </div>

      {plan.changes.length > 0 ? (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">
            {t('plan.changesTitle', {
              target: plan.target.version,
              current: plan.current_version,
            })}
          </p>
          <div className="max-h-48 space-y-3 overflow-auto rounded-md bg-muted px-3 py-2">
            {plan.changes.map((c) => (
              <div key={c.version}>
                <p className="text-xs font-medium text-foreground">
                  {c.version}
                </p>
                <ReleaseNotes markdown={c.notes} />
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {!plan.one_click && !plan.blocked && plan.one_click_reason ? (
        <p className="text-xs text-muted-foreground">
          {t('plan.oneClick.unavailable', { reason: plan.one_click_reason })}
        </p>
      ) : null}

      {!plan.blocked ? <FixCli steps={rollbackSteps(plan, t)} /> : null}
    </div>
  )
}

export function RollbackPlanDialog({
  version,
  onClose,
}: {
  version: string | null
  onClose: () => void
}) {
  const { t } = useTranslation('updates')
  const { data, isPending, isError, error, refetch, isFetching } = useQuery(
    rollbackPlanQueryOptions(version),
  )
  const start = useStartRollback()
  const [confirming, setConfirming] = useState(false)
  return (
    <Dialog
      open={version !== null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t('plan.title', { version: version ?? '' })}
          </DialogTitle>
          <DialogDescription>{t('plan.description')}</DialogDescription>
        </DialogHeader>
        {isPending ? (
          <p className="text-sm text-muted-foreground">{t('plan.loading')}</p>
        ) : isError ? (
          <div className="space-y-2">
            <p role="alert" className="text-sm text-destructive">
              {t('plan.loadError', { message: error.message })}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                void refetch()
              }}
            >
              {t('history.retry')}
            </Button>
          </div>
        ) : (
          <PlanBody plan={data} />
        )}
        <DialogFooter>
          {data?.one_click && version ? (
            <Button
              type="button"
              variant="destructive"
              onClick={() => {
                start.reset()
                setConfirming(true)
              }}
            >
              {t('plan.oneClick.button')}
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            disabled={isFetching}
            onClick={() => {
              void refetch()
            }}
          >
            {isFetching ? t('plan.rechecking') : t('plan.recheck')}
          </Button>
          <Button type="button" variant="outline" onClick={onClose}>
            {t('plan.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
      {version ? (
        <ConfirmDialog
          open={confirming}
          onOpenChange={setConfirming}
          title={t('plan.oneClick.confirmTitle', { version })}
          description={t('plan.oneClick.confirmBody')}
          confirmLabel={t('plan.oneClick.confirm')}
          pendingLabel={t('plan.oneClick.pending')}
          tone="destructive"
          requireTyped={version}
          pending={start.isPending}
          error={start.isError ? start.error.message : null}
          onConfirm={() => {
            start.mutate(version, {
              onSuccess: () => {
                setConfirming(false)
                toast.add({
                  title: t('plan.oneClick.started', { version }),
                  type: 'success',
                })
                onClose()
              },
            })
          }}
        />
      ) : null}
    </Dialog>
  )
}
