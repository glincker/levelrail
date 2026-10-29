import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  LoaderCircleGlassIcon,
  ShieldCheckGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '../../../lib/format'
import { useBackupTargetsOptional } from '../../../queries/backupTargets'
import {
  useBackupHistory,
  useTriggerBackup,
} from '../../../queries/backupHistory'
import { useBackupHealth } from '../../../queries/backupProtection'
import { useSetDatabaseBackupSchedule } from '../../../queries/databases'
import type { DatabaseResource } from '../../../types/databaseDetail'
import {
  canSubmitStep,
  drillState,
  isTerminal,
  parseRetain,
  presetToCron,
  type SchedulePreset,
} from './backupSetupHelpers'

type Step = 'target' | 'schedule' | 'retention' | 'run'

const STEPS: Step[] = ['target', 'schedule', 'retention', 'run']
const PRESETS: SchedulePreset[] = ['daily', 'weekly', 'custom']
const DEFAULT_RETAIN = '7'

function DrillStatus({ database }: { database: string }) {
  const { t } = useTranslation('databases')
  const health = useBackupHealth()
  const resource = health.data?.resources.find(
    (r) => r.kind === 'database' && r.resource_name === database,
  )
  const state = drillState(resource?.last_drill)
  const Icon =
    state === 'passed'
      ? ShieldCheckGlassIcon
      : state === 'failed'
        ? CircleAlertGlassIcon
        : LoaderCircleGlassIcon
  return (
    <p
      className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground"
      data-testid="drill-status"
    >
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      {t(`backupSetup.drill.${state}`)}
    </p>
  )
}

function RunStep({
  database,
  started,
}: {
  database: string
  started: boolean
}) {
  const { t } = useTranslation('databases')
  const history = useBackupHistory(database)
  const latest = started ? history.data?.[0] : undefined
  if (!started || !latest || latest.status === 'running') {
    return (
      <p className="flex items-center gap-2 text-sm text-foreground">
        <LoaderCircleGlassIcon
          className="size-4.5 animate-spin"
          aria-hidden="true"
        />
        {t('backupSetup.run.progress')}
      </p>
    )
  }
  if (latest.status === 'failed') {
    return (
      <p
        className="flex items-start gap-2 text-sm text-destructive"
        role="alert"
      >
        <CircleAlertGlassIcon
          className="mt-0.5 size-4.5 text-tone-danger"
          aria-hidden="true"
        />
        {t('backupSetup.run.failed', { error: latest.error ?? '' })}
      </p>
    )
  }
  return (
    <p className="flex items-center gap-2 text-sm text-foreground">
      <CircleCheckGlassIcon
        className="size-4.5 text-success"
        aria-hidden="true"
      />
      {t('backupSetup.run.done', {
        size: formatBytes(latest.size_bytes ?? 0),
      })}
    </p>
  )
}

export function BackupSetupDialog({
  database,
  open,
  onOpenChange,
}: {
  database: DatabaseResource
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('databases')
  const targets = useBackupTargetsOptional().data ?? []
  const setSchedule = useSetDatabaseBackupSchedule()
  const trigger = useTriggerBackup(database.name)
  const history = useBackupHistory(database.name)
  const [step, setStep] = useState<Step>('target')
  const [targetId, setTargetId] = useState(targets[0]?.id ?? '')
  const [preset, setPreset] = useState<SchedulePreset>('daily')
  const [custom, setCustom] = useState('0 */6 * * *')
  const [retain, setRetain] = useState(DEFAULT_RETAIN)
  const [started, setStarted] = useState(false)

  const cron = presetToCron(preset, custom)
  const effectiveTarget = targetId || targets[0]?.id || ''
  const values = { targetId: effectiveTarget, cron, retain }
  const latest = started ? history.data?.[0] : undefined
  const finished = isTerminal(latest?.status)

  function next() {
    setStep(STEPS[STEPS.indexOf(step) + 1] ?? 'run')
  }

  function saveAndRun() {
    const keep = parseRetain(retain)
    if (keep === null) return
    setStep('run')
    setSchedule.mutate(
      {
        name: database.name,
        targetId: effectiveTarget,
        schedule: cron,
        retain: keep,
        retainDays: database.backup_retain_days ?? 0,
      },
      {
        onSuccess: () => {
          trigger.mutate(effectiveTarget, {
            onSuccess: () => {
              setStarted(true)
              toast.add({
                title: t('backupSetup.saved', { name: database.name }),
                type: 'success',
              })
            },
          })
        },
      },
    )
  }

  const runError = setSchedule.error ?? trigger.error

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90dvh] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t('backupSetup.title', { name: database.name })}
          </DialogTitle>
          <DialogDescription>
            {t('backupSetup.stepOf', {
              current: STEPS.indexOf(step) + 1,
              total: STEPS.length,
            })}
          </DialogDescription>
        </DialogHeader>

        <div className="min-w-0 space-y-4 overflow-y-auto">
          <DrillStatus database={database.name} />

          {step === 'target' ? (
            targets.length === 0 ? (
              <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4 text-center">
                <UploadGlassIcon
                  className="mx-auto size-5.5 text-muted-foreground"
                  aria-hidden="true"
                />
                <p className="text-sm text-muted-foreground">
                  {t('backupSetup.target.none')}
                </p>
                <Link
                  to="/settings/backup-targets"
                  className={buttonVariants({ size: 'sm' })}
                >
                  {t('backupSetup.target.connect')}
                </Link>
              </div>
            ) : (
              <fieldset className="space-y-2">
                <legend className="text-sm font-medium">
                  {t('backupSetup.target.label')}
                </legend>
                {targets.map((target) => (
                  <label
                    key={target.id}
                    className="flex cursor-pointer items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
                  >
                    <input
                      type="radio"
                      name="backup-target"
                      checked={effectiveTarget === target.id}
                      onChange={() => {
                        setTargetId(target.id)
                      }}
                    />
                    <span className="font-medium">{target.name}</span>
                    <span className="text-xs text-muted-foreground">
                      {target.bucket}
                    </span>
                  </label>
                ))}
              </fieldset>
            )
          ) : null}

          {step === 'schedule' ? (
            <fieldset className="space-y-2">
              <legend className="text-sm font-medium">
                {t('backupSetup.schedule.label')}
              </legend>
              {PRESETS.map((p) => (
                <label
                  key={p}
                  className="flex cursor-pointer items-center gap-2 rounded-md border border-border px-3 py-2 text-sm"
                >
                  <input
                    type="radio"
                    name="backup-preset"
                    checked={preset === p}
                    onChange={() => {
                      setPreset(p)
                    }}
                  />
                  {t(`backupSetup.schedule.${p}`)}
                </label>
              ))}
              {preset === 'custom' ? (
                <div className="space-y-1">
                  <Input
                    value={custom}
                    onChange={(e) => {
                      setCustom(e.target.value)
                    }}
                    aria-label={t('backupSetup.schedule.cronLabel')}
                    aria-invalid={!canSubmitStep('schedule', values)}
                  />
                  <p className="text-xs text-muted-foreground">
                    {t('backupSetup.schedule.cronHint')}
                  </p>
                </div>
              ) : null}
            </fieldset>
          ) : null}

          {step === 'retention' ? (
            <label className="block space-y-1 text-sm font-medium">
              {t('backupSetup.retention.label')}
              <Input
                inputMode="numeric"
                value={retain}
                onChange={(e) => {
                  setRetain(e.target.value)
                }}
                aria-invalid={!canSubmitStep('retention', values)}
              />
              <span className="block text-xs font-normal text-muted-foreground">
                {t('backupSetup.retention.hint')}
              </span>
            </label>
          ) : null}

          {step === 'run' ? (
            <div className="space-y-2">
              {runError ? (
                <p className="text-sm text-destructive" role="alert">
                  {runError.message}
                </p>
              ) : (
                <RunStep database={database.name} started={started} />
              )}
            </div>
          ) : null}
        </div>

        <DialogFooter className="flex-wrap">
          {step === 'run' ? (
            <Button
              type="button"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {finished || runError
                ? t('backupSetup.close')
                : t('backupSetup.runInBackground')}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  onOpenChange(false)
                }}
              >
                {t('backupSetup.cancel')}
              </Button>
              {step === 'retention' ? (
                <Button
                  type="button"
                  disabled={!canSubmitStep('retention', values)}
                  onClick={saveAndRun}
                >
                  {t('backupSetup.saveAndRun')}
                </Button>
              ) : (
                <Button
                  type="button"
                  disabled={!canSubmitStep(step, values)}
                  onClick={next}
                >
                  {t('backupSetup.next')}
                </Button>
              )}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
