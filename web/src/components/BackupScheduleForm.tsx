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
import type { BackupTarget } from '../types/backupTarget'
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
  const scheduled = !!database.backup_schedule

  function handleSubmit(values: ScheduleFormValues) {
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
  }

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
    <BackupScheduleFormView
      idPrefix="backup-schedule"
      values={toFieldValues(database)}
      scheduled={scheduled}
      summary={
        scheduled
          ? scheduleSummary(database)
          : 'No recurring backup configured for this database.'
      }
      targets={targets}
      submitPending={setSchedule.isPending}
      submitError={setSchedule.isError ? setSchedule.error.message : null}
      clearPending={clearSchedule.isPending}
      onSubmit={handleSubmit}
      onClear={handleClear}
    />
  )
}

// Shared between BackupScheduleForm and VolumeBackupScheduleForm: same
// daily/weekly/custom-cron UI over the same field grammar (lib/cronSchedule.ts)
// for both resource kinds. Each caller owns its own data source and
// mutation wiring (the payload shapes differ: a database schedule call
// carries the database name and camelCase keys, a volume one is already
// scoped to app/volume and uses the API's own snake_case keys) and hands
// this view plain callbacks instead.
export function BackupScheduleFormView({
  idPrefix,
  values,
  scheduled,
  summary,
  targets,
  submitPending,
  submitError,
  clearPending,
  onSubmit,
  onClear,
}: {
  idPrefix: string
  values: ScheduleFormValues
  scheduled: boolean
  summary: string
  targets: BackupTarget[]
  submitPending: boolean
  submitError: string | null
  clearPending: boolean
  onSubmit: (values: ScheduleFormValues) => void
  onClear: () => void
}) {
  const { control, register, handleSubmit, watch, formState } =
    useForm<ScheduleFormValues>({
      resolver: zodResolver(scheduleSchema),
      values,
      resetOptions: { keepDirtyValues: true },
    })

  const frequency = watch('frequency')

  // No backup targets to schedule against yet: the trigger row already
  // shows the "connect a target" prompt in this same card, so this
  // section stays hidden rather than duplicating that message.
  if (targets.length === 0) {
    return null
  }

  const submit = handleSubmit(onSubmit)

  return (
    <div className="space-y-3 rounded-lg border border-border p-4">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-medium text-foreground">
            <ClockIcon className="size-4" aria-hidden="true" />
            Scheduled backups
          </h3>
          <p className="text-sm text-muted-foreground">{summary}</p>
        </div>
        {scheduled ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onClear}
            disabled={clearPending}
          >
            <TrashIcon className="size-3.5" aria-hidden="true" />
            {clearPending ? 'Removing...' : 'Remove schedule'}
          </Button>
        ) : null}
      </div>

      <form
        onSubmit={(e) => {
          void submit(e)
        }}
        className="grid grid-cols-1 gap-3 sm:grid-cols-2"
      >
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-target`}>Backup target</FieldLabel>
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
                <SelectTrigger id={`${idPrefix}-target`} className="w-full">
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
          <FieldLabel htmlFor={`${idPrefix}-frequency`}>Frequency</FieldLabel>
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
                <SelectTrigger id={`${idPrefix}-frequency`} className="w-full">
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
            <FieldLabel htmlFor={`${idPrefix}-weekday`}>
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
                  <SelectTrigger id={`${idPrefix}-weekday`} className="w-full">
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
            <FieldLabel htmlFor={`${idPrefix}-time`}>Time</FieldLabel>
            <Input id={`${idPrefix}-time`} type="time" {...register('time')} />
            <FieldError errors={[formState.errors.time]} />
          </Field>
        ) : (
          <Field className="sm:col-span-2">
            <FieldLabel htmlFor={`${idPrefix}-cron`}>
              Cron expression
            </FieldLabel>
            <Input
              id={`${idPrefix}-cron`}
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
          <FieldLabel htmlFor={`${idPrefix}-retain`}>
            Keep last N backups
          </FieldLabel>
          <Input
            id={`${idPrefix}-retain`}
            inputMode="numeric"
            className="max-w-32"
            {...register('retain')}
          />
          <FieldDescription>0 means no limit on count.</FieldDescription>
          <FieldError errors={[formState.errors.retain]} />
        </Field>

        <Field>
          <FieldLabel htmlFor={`${idPrefix}-retain-days`}>
            Delete backups older than (days)
          </FieldLabel>
          <Input
            id={`${idPrefix}-retain-days`}
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
          <Button type="submit" size="sm" disabled={submitPending}>
            {submitPending
              ? 'Saving...'
              : scheduled
                ? 'Update schedule'
                : 'Enable scheduled backups'}
          </Button>
        </div>
      </form>

      {submitError ? (
        <Alert variant="destructive">
          <AlertDescription>{submitError}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
