import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
  type RollbackPlan,
} from '../../queries/releases'
import type { UpgradeCheckStatus } from '../../queries/updates'
import { CopyCommand } from './CopyCommand'
import { ReleaseNotes } from './ReleaseNotes'

const STATUS_TONE: Record<UpgradeCheckStatus, Tone> = {
  ok: 'success',
  warn: 'warning',
  fail: 'danger',
  unknown: 'neutral',
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
      <WarningIcon />
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

      {!plan.blocked ? (
        <div className="space-y-3">
          {plan.fetch_command ? (
            <CopyCommand
              label={t('plan.commands.fetch')}
              command={plan.fetch_command}
            />
          ) : null}
          {plan.restore_required ? (
            plan.restore_command ? (
              <CopyCommand
                label={t('plan.commands.restore')}
                command={plan.restore_command}
              />
            ) : null
          ) : (
            <CopyCommand
              label={t('plan.commands.apply', { version: plan.target.version })}
              command={plan.command}
            />
          )}
        </div>
      ) : null}
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
  const { data, isPending, isError, error, refetch } = useQuery(
    rollbackPlanQueryOptions(version),
  )
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
          <Button type="button" variant="outline" onClick={onClose}>
            {t('plan.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
