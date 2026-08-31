import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { ClockIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useBackupTargetsOptional } from '../queries/backupTargets'
import {
  useClearDatabaseBackupSchedule,
  useSetDatabaseBackupSchedule,
} from '../queries/databases'
import type { DatabaseResource } from '../types/databaseDetail'
import {
  WEEKDAY_LABEL,
  fromCron,
  scheduleRetentionSummary,
  scheduleSchema,
  toCron,
  type ScheduleFormValues,
} from '../lib/cronSchedule'

function scheduleSummary(database: DatabaseResource): string {
  const retention = scheduleRetentionSummary(
    database.backup_retain,
    database.backup_retain_days,
  )
  return `Runs on schedule "${database.backup_schedule}", ${retention}.`
}

function toFieldValues(database: DatabaseResource): ScheduleFormValues {
  return {
    targetId: database.backup_target_id ?? '',
    retain: String(database.backup_retain ?? 7),
    retainDays: String(database.backup_retain_days ?? 0),
    ...fromCron(database.backup_schedule),
  }
}

export function BackupScheduleForm({
  database,
}: {
  database: DatabaseResource
}) {
  const targets = useBackupTargetsOptional().data ?? []
  const setSchedule = useSetDatabaseBackupSchedule()
  const clearSchedule = useClearDatabaseBackupSchedule()
  const { control, register, handleSubmit, watch, formState } =
    useForm<ScheduleFormValues>({
      resolver: zodResolver(scheduleSchema),
      values: toFieldValues(database),
      resetOptions: { keepDirtyValues: true },
    })

  const frequency = watch('frequency')
  const scheduled = !!database.backup_schedule

  // No backup targets to schedule against yet: TriggerBackupRow already
  // shows the "connect a target" prompt in this same card, so this
  // section stays hidden rather than duplicating that message.
  if (targets.length === 0) {
    return null
  }

  const onSubmit = handleSubmit((values) => {
    setSchedule.mutate(
      {
        name: database.name,
        targetId: values.targetId,
        schedule: toCron(values),
        retain: Math.round(Number(values.retain)),
        retainDays: Math.round(Number(values.retainDays)),
      },
      {
        onSuccess: () => {
          toast.add({ title: 'Backup schedule saved.', type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: 'Could not save backup schedule.',
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  })

  function handleClear() {
    clearSchedule.mutate(database.name, {
      onSuccess: () => {
        toast.add({ title: 'Backup schedule removed.', type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not remove backup schedule.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-medium text-foreground">
            <ClockIcon className="size-4" aria-hidden="true" />
            Scheduled backups
          </h3>
          <p className="text-sm text-muted-foreground">
            {scheduled
              ? scheduleSummary(database)
              : 'No recurring backup configured for this database.'}
          </p>
        </div>
        {scheduled ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleClear}
            disabled={clearSchedule.isPending}
          >
            <TrashIcon className="size-3.5" aria-hidden="true" />
            {clearSchedule.isPending ? 'Removing...' : 'Remove schedule'}
          </Button>
        ) : null}
      </div>

      <form
        onSubmit={(e) => {
          void onSubmit(e)
        }}
        className="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <Field>
          <FieldLabel htmlFor="backup-schedule-target">
            Backup target
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
                <SelectTrigger id="backup-schedule-target" className="w-full">
                  <SelectValue placeholder="Choose a backup target..." />
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

        <Field>
          <FieldLabel htmlFor="backup-schedule-frequency">Frequency</FieldLabel>
          <Controller
            control={control}
            name="frequency"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value: string | null) => {
                  field.onChange(value ?? 'daily')
                }}
              >
                <SelectTrigger
                  id="backup-schedule-frequency"
                  className="w-full"
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="daily">Daily</SelectItem>
                  <SelectItem value="weekly">Weekly</SelectItem>
                  <SelectItem value="custom">Custom cron expression</SelectItem>
                </SelectContent>
              </Select>
            )}
          />
        </Field>

        {frequency === 'weekly' ? (
          <Field>
            <FieldLabel htmlFor="backup-schedule-weekday">
              Day of week
            </FieldLabel>
            <Controller
              control={control}
              name="weekday"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={(value: string | null) => {
                    field.onChange(value ?? '0')
                  }}
                >
                  <SelectTrigger
                    id="backup-schedule-weekday"
                    className="w-full"
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(WEEKDAY_LABEL).map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>
        ) : null}

        {frequency !== 'custom' ? (
          <Field>
            <FieldLabel htmlFor="backup-schedule-time">Time</FieldLabel>
            <Input
              id="backup-schedule-time"
              type="time"
              {...register('time')}
            />
            <FieldError errors={[formState.errors.time]} />
          </Field>
        ) : (
          <Field className="sm:col-span-2">
            <FieldLabel htmlFor="backup-schedule-cron">
              Cron expression
            </FieldLabel>
            <Input
              id="backup-schedule-cron"
              placeholder="0 3 * * *"
              className="font-mono"
              {...register('customCron')}
            />
            <FieldDescription>
              Standard 5-field cron: minute hour day-of-month month day-of-week.
            </FieldDescription>
            <FieldError errors={[formState.errors.customCron]} />
          </Field>
        )}

        <Field>
          <FieldLabel htmlFor="backup-schedule-retain">
            Keep last N backups
          </FieldLabel>
          <Input
            id="backup-schedule-retain"
            inputMode="numeric"
            className="max-w-32"
            {...register('retain')}
          />
          <FieldDescription>0 means no limit on count.</FieldDescription>
          <FieldError errors={[formState.errors.retain]} />
        </Field>

        <Field>
          <FieldLabel htmlFor="backup-schedule-retain-days">
            Delete backups older than (days)
          </FieldLabel>
          <Input
            id="backup-schedule-retain-days"
            inputMode="numeric"
            className="max-w-32"
            {...register('retainDays')}
          />
          <FieldDescription>
            0 means no age limit. Applies independently of the count above.
          </FieldDescription>
          <FieldError errors={[formState.errors.retainDays]} />
        </Field>

        <div className="flex items-end sm:col-span-2">
          <Button type="submit" size="sm" disabled={setSchedule.isPending}>
            {setSchedule.isPending
              ? 'Saving...'
              : scheduled
                ? 'Update schedule'
                : 'Enable scheduled backups'}
          </Button>
        </div>
      </form>

      {setSchedule.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{setSchedule.error.message}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
