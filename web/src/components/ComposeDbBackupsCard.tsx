import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import {
  ChevronDownGlassIcon,
  ClockGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useAbilities } from '../hooks/useAbilities'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import {
  useClearComposeDbSchedule,
  useComposeDbRuns,
  useComposeDbServices,
  useSetComposeDbSchedule,
  useTriggerComposeDbBackup,
} from '../queries/composeDbBackups'
import {
  fromCron,
  scheduleRetentionSummary,
  scheduleSchema,
  toCron,
  type ScheduleFormValues,
} from '../lib/cronSchedule'
import type { BackupTarget } from '../types/backupTarget'
import type {
  ComposeDbBackupRun,
  ComposeDbService,
} from '../types/composeDbBackups'
import { CronScheduleFields } from './CronScheduleFields'

function toFieldValues(svc: ComposeDbService): ScheduleFormValues {
  return {
    targetId: svc.target_id ?? '',
    // Server omits retain when 0 (keep all); only default to 7 for a new schedule.
    retain: String(svc.scheduled ? (svc.retain ?? 0) : 7),
    retainDays: String(svc.retain_days ?? 0),
    ...fromCron(svc.schedule),
  }
}

function RunsList({ app, service }: { app: string; service: string }) {
  const { t } = useTranslation('backups')
  const runs = useComposeDbRuns(app, service, true)
  if (runs.isError) {
    return (
      <p className="text-sm text-destructive">
        {t('composeDb.runsFailed', { error: runs.error.message })}
      </p>
    )
  }
  const rows: ComposeDbBackupRun[] = runs.data ?? []
  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('composeDb.noRuns')}</p>
    )
  }
  return (
    <ul className="divide-y divide-border rounded-md border border-border text-sm">
      {rows.map((run) => (
        <li
          key={run.id}
          className="flex flex-wrap items-center gap-2 px-3 py-2"
        >
          <Badge
            variant={
              run.status === 'failed'
                ? 'destructive'
                : run.status === 'succeeded'
                  ? 'success'
                  : 'outline'
            }
          >
            {run.status === 'running' ||
            run.status === 'succeeded' ||
            run.status === 'failed'
              ? t(`composeDb.status.${run.status}`)
              : run.status}
          </Badge>
          <time
            dateTime={run.started_at}
            className="text-muted-foreground tabular-nums"
          >
            {new Date(run.started_at).toLocaleString()}
          </time>
          {run.error ? (
            <span className="min-w-0 break-words text-destructive">
              {run.error}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

function ServiceRow({
  app,
  svc,
  targets,
  canWrite,
}: {
  app: string
  svc: ComposeDbService
  targets: BackupTarget[]
  canWrite: boolean
}) {
  const { t } = useTranslation('backups')
  const setSchedule = useSetComposeDbSchedule(app, svc.service)
  const clearSchedule = useClearComposeDbSchedule(app, svc.service)
  const trigger = useTriggerComposeDbBackup(app, svc.service)
  const [enabled, setEnabled] = useState(svc.scheduled ? svc.enabled : true)
  const [showRuns, setShowRuns] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const { control, register, handleSubmit, watch, formState } =
    useForm<ScheduleFormValues>({
      resolver: zodResolver(scheduleSchema),
      values: toFieldValues(svc),
      resetOptions: { keepDirtyValues: true },
    })
  const targetId = watch('targetId')
  const id = `compose-db-${svc.service}`

  const submit = handleSubmit((values) => {
    setSchedule.mutate(
      {
        target_id: values.targetId,
        schedule: toCron(values),
        retain: Math.round(Number(values.retain)),
        retain_days: Math.round(Number(values.retainDays)),
        enabled,
      },
      {
        onSuccess: () =>
          toast.add({ title: t('composeDb.saved'), type: 'success' }),
        onError: (error) =>
          toast.add({
            title: t('composeDb.saveFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  })

  function handleRemove() {
    clearSchedule.mutate(undefined, {
      onSuccess: () => {
        setConfirmRemove(false)
        toast.add({ title: t('composeDb.removed'), type: 'success' })
      },
      onError: (error) =>
        toast.add({
          title: t('composeDb.removeFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  function handleBackupNow() {
    trigger.mutate(
      { target_id: targetId },
      {
        onSuccess: () => {
          setShowRuns(true)
          toast.add({ title: t('composeDb.started'), type: 'success' })
        },
        onError: (error) =>
          toast.add({
            title: t('composeDb.startFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  const summary = svc.scheduled
    ? t('composeDb.scheduledSummary', {
        schedule: svc.schedule,
        retention: scheduleRetentionSummary(svc.retain, svc.retain_days),
      })
    : t('composeDb.notScheduled')

  return (
    <li className="space-y-3 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-foreground">{svc.service}</span>
        <Badge variant="outline">{svc.engine}</Badge>
        {svc.scheduled && !svc.enabled ? (
          <Badge variant="muted">{t('composeDb.paused')}</Badge>
        ) : null}
        <span className="min-w-0 text-sm break-words text-muted-foreground">
          {summary}
        </span>
      </div>

      {canWrite ? (
        <form
          onSubmit={(e) => {
            void submit(e)
          }}
          className="grid grid-cols-1 items-start gap-3 sm:grid-cols-2 lg:grid-cols-4"
        >
          <Field>
            <FieldLabel htmlFor={`${id}-target`}>
              {t('composeDb.target')}
            </FieldLabel>
            <Controller
              control={control}
              name="targetId"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={(value: string | null) => {
                    field.onChange(value ?? '')
                  }}
                >
                  <SelectTrigger id={`${id}-target`} className="w-full">
                    <SelectValue
                      placeholder={t('composeDb.targetPlaceholder')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {targets.map((target) => (
                      <SelectItem key={target.id} value={target.id}>
                        {target.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
            <FieldError errors={[formState.errors.targetId]} />
          </Field>
          <CronScheduleFields<ScheduleFormValues>
            idPrefix={id}
            control={control}
            register={register}
            formState={formState}
            frequency={watch('frequency')}
          />
          <Field>
            <FieldLabel htmlFor={`${id}-retain`}>
              {t('composeDb.retain')}
            </FieldLabel>
            <Input
              id={`${id}-retain`}
              inputMode="numeric"
              className="max-w-32"
              {...register('retain')}
            />
            <FieldError errors={[formState.errors.retain]} />
          </Field>
          <Field>
            <FieldLabel htmlFor={`${id}-retain-days`}>
              {t('composeDb.retainDays')}
            </FieldLabel>
            <Input
              id={`${id}-retain-days`}
              inputMode="numeric"
              className="max-w-32"
              {...register('retainDays')}
            />
            <FieldError errors={[formState.errors.retainDays]} />
          </Field>
          <div className="flex flex-wrap items-center gap-2 sm:col-span-2 lg:col-span-4">
            <label className="flex items-center gap-2 text-sm">
              <Switch
                size="sm"
                checked={enabled}
                onCheckedChange={setEnabled}
                aria-label={t('composeDb.enabled')}
              />
              {t('composeDb.enabled')}
            </label>
            <Button type="submit" size="sm" disabled={setSchedule.isPending}>
              {setSchedule.isPending
                ? t('composeDb.saving')
                : t('composeDb.save')}
            </Button>
            {svc.scheduled ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setConfirmRemove(true)}
              >
                <TrashGlassIcon className="size-5" aria-hidden="true" />
                {t('composeDb.remove')}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!targetId || trigger.isPending}
              onClick={handleBackupNow}
            >
              <ClockGlassIcon className="size-5" aria-hidden="true" />
              {trigger.isPending
                ? t('composeDb.starting')
                : t('composeDb.backUpNow')}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-expanded={showRuns}
              onClick={() => setShowRuns((v) => !v)}
            >
              <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
              {showRuns ? t('composeDb.runsHide') : t('composeDb.runs')}
            </Button>
          </div>
        </form>
      ) : (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={showRuns}
          onClick={() => setShowRuns((v) => !v)}
        >
          <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
          {showRuns ? t('composeDb.runsHide') : t('composeDb.runs')}
        </Button>
      )}

      {setSchedule.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{setSchedule.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {showRuns ? <RunsList app={app} service={svc.service} /> : null}

      <ConfirmDialog
        open={confirmRemove}
        onOpenChange={setConfirmRemove}
        title={t('composeDb.removeTitle', { service: svc.service })}
        description={t('composeDb.removeDescription')}
        confirmLabel={t('composeDb.remove')}
        pendingLabel={t('composeDb.removing')}
        tone="destructive"
        pending={clearSchedule.isPending}
        onConfirm={handleRemove}
      />
    </li>
  )
}

export function ComposeDbBackupsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('backups')
  const services = useComposeDbServices(appName)
  const targetsQuery = useBackupTargetsOptional()
  const { can } = useAbilities()
  const list = services.data ?? []
  if (list.length === 0) return null
  const targets = targetsQuery.data ?? []
  const canWrite = can('write:sensitive')

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('composeDb.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {canWrite && targets.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('composeDb.noTargets')}{' '}
            <Link
              to="/settings/backup-targets"
              className="text-primary underline underline-offset-2"
            >
              {t('composeDb.connectTarget')}
            </Link>
          </p>
        ) : null}
        <ul className="divide-y divide-border">
          {list.map((svc) => (
            <ServiceRow
              key={svc.service}
              app={appName}
              svc={svc}
              targets={targets}
              canWrite={canWrite}
            />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
