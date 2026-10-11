import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'
import {
  PencilSimpleIcon,
  PlusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useCreateScheduledTask,
  useUpdateScheduledTask,
} from '../queries/scheduledTasks'
import type {
  ScheduledTask,
  ScheduledTaskCatchUp,
  ScheduledTaskConcurrencyPolicy,
  ScheduledTaskMode,
  ScheduledTaskRequest,
} from '../types/scheduledTasks'
import { fromCron, refineCronFields, toCron } from '../lib/cronSchedule'
import { CronScheduleFields } from './CronScheduleFields'

const optionalCount = z.string().trim().regex(/^\d*$/, 'whole number')

const jobSchema = z
  .object({
    name: z
      .string()
      .trim()
      .regex(/^([a-z0-9][a-z0-9_-]{0,62})?$/, 'invalid'),
    command: z.string().trim().min(1, 'required'),
    mode: z.enum(['fresh', 'exec']),
    timezone: z.string().trim().min(1, 'required'),
    frequency: z.enum(['daily', 'weekly', 'custom']),
    time: z.string().trim(),
    weekday: z.string().trim(),
    customCron: z.string().trim(),
    enabled: z.boolean(),
    concurrencyPolicy: z.enum(['allow', 'forbid', 'replace']),
    timeoutSeconds: optionalCount,
    maxRetries: optionalCount,
    retryBackoffSeconds: optionalCount,
    catchUp: z.enum(['skip', 'run_once']),
    catchUpLookbackSeconds: optionalCount,
    memoryLimitMb: optionalCount,
    cpuLimitMillis: optionalCount,
    warnAfterSeconds: optionalCount,
  })
  .superRefine(refineCronFields)

type JobFormValues = z.infer<typeof jobSchema>

const DEFAULT_VALUES: JobFormValues = {
  name: '',
  command: '',
  mode: 'fresh',
  timezone: 'UTC',
  enabled: true,
  concurrencyPolicy: 'forbid',
  timeoutSeconds: '',
  maxRetries: '',
  retryBackoffSeconds: '',
  catchUp: 'skip',
  catchUpLookbackSeconds: '',
  memoryLimitMb: '',
  cpuLimitMillis: '',
  warnAfterSeconds: '',
  ...fromCron('0 3 * * *'),
}

// A command made of ["sh", "-c", script] shows as the bare script; any
// other argv shows space-joined, an honest if lossy display.
function commandToDisplay(command: string[]): string {
  if (command.length === 3 && command[0] === 'sh' && command[1] === '-c') {
    return command[2] ?? ''
  }
  return command.join(' ')
}

function displayToCommand(value: string): string[] {
  return ['sh', '-c', value.trim()]
}

function numStr(v: number | undefined): string {
  return v ? String(v) : ''
}

function numOrUndefined(v: string): number | undefined {
  return v === '' ? undefined : Number(v)
}

function toFormValues(task: ScheduledTask): JobFormValues {
  return {
    name: task.name ?? '',
    command: commandToDisplay(task.command),
    mode: task.mode ?? 'exec',
    timezone: task.timezone || 'UTC',
    enabled: task.enabled,
    concurrencyPolicy: task.concurrency_policy,
    timeoutSeconds: numStr(task.timeout_seconds),
    maxRetries: numStr(task.max_retries),
    retryBackoffSeconds: numStr(task.retry_backoff_seconds),
    catchUp: task.catch_up ?? 'skip',
    catchUpLookbackSeconds: numStr(task.catch_up_lookback_seconds),
    memoryLimitMb: numStr(task.memory_limit_mb),
    cpuLimitMillis: numStr(task.cpu_limit_millis),
    warnAfterSeconds: numStr(task.warn_after_seconds),
    ...fromCron(task.schedule),
  }
}

export function JobDialog({
  appName,
  task,
}: {
  appName: string
  task?: ScheduledTask
}) {
  const { t } = useTranslation('jobs')
  const [open, setOpen] = useState(false)
  const isEdit = task !== undefined
  const createTask = useCreateScheduledTask(appName)
  const updateTask = useUpdateScheduledTask(appName)
  const mutation = isEdit ? updateTask : createTask
  const specManaged = task?.source === 'spec'

  const defaultValues = task ? toFormValues(task) : DEFAULT_VALUES
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<JobFormValues>({
      resolver: zodResolver(jobSchema),
      defaultValues,
    })

  const frequency = watch('frequency')
  const mode = watch('mode')
  const catchUp = watch('catchUp')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req: ScheduledTaskRequest = {
      name: values.name || undefined,
      mode: values.mode,
      command: displayToCommand(values.command),
      schedule: toCron(values),
      timezone: values.timezone,
      enabled: values.enabled,
      concurrency_policy: values.concurrencyPolicy,
      timeout_seconds: numOrUndefined(values.timeoutSeconds),
      max_retries: numOrUndefined(values.maxRetries),
      retry_backoff_seconds: numOrUndefined(values.retryBackoffSeconds),
      catch_up: values.catchUp,
      catch_up_lookback_seconds: numOrUndefined(values.catchUpLookbackSeconds),
      memory_limit_mb: numOrUndefined(values.memoryLimitMb),
      cpu_limit_millis: numOrUndefined(values.cpuLimitMillis),
      warn_after_seconds: numOrUndefined(values.warnAfterSeconds),
    }
    const onSuccess = () => {
      handleOpenChange(false)
      toast.add({
        title: isEdit ? t('dialog.updated') : t('dialog.created'),
        type: 'success',
      })
    }
    if (isEdit) {
      updateTask.mutate({ id: task.id, req }, { onSuccess })
    } else {
      createTask.mutate(req, { onSuccess })
    }
  })

  const numberField = (
    id: string,
    name: keyof JobFormValues,
    label: string,
    hint?: string,
  ) => (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input id={id} inputMode="numeric" {...register(name)} />
      {hint ? <FieldDescription>{hint}</FieldDescription> : null}
      <FieldError
        errors={[
          formState.errors[name] && { message: t('dialog.wholeNumber') },
        ]}
      />
    </Field>
  )

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          isEdit ? (
            <Button variant="outline" size="sm" disabled={specManaged}>
              <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
              {t('dialog.edit')}
            </Button>
          ) : (
            <Button size="sm">
              <PlusCircleIcon className="size-3.5" aria-hidden="true" />
              {t('dialog.createTrigger')}
            </Button>
          )
        }
      />
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('dialog.editTitle') : t('dialog.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="job-name">{t('dialog.name')}</FieldLabel>
            <Input
              id="job-name"
              placeholder={t('dialog.namePlaceholder')}
              {...register('name')}
            />
            <FieldDescription>{t('dialog.nameHint')}</FieldDescription>
            <FieldError
              errors={[
                formState.errors.name && { message: t('dialog.nameInvalid') },
              ]}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="job-command">{t('dialog.command')}</FieldLabel>
            <Input
              id="job-command"
              placeholder={t('dialog.commandPlaceholder')}
              {...register('command')}
            />
            <FieldDescription>{t('dialog.commandHint')}</FieldDescription>
            <FieldError
              errors={[
                formState.errors.command && {
                  message: t('dialog.commandRequired'),
                },
              ]}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="job-mode">{t('dialog.modeLabel')}</FieldLabel>
            <Controller
              control={control}
              name="mode"
              render={({ field }) => (
                <Select<ScheduledTaskMode>
                  value={field.value}
                  onValueChange={(value) => {
                    field.onChange(value ?? 'fresh')
                  }}
                >
                  <SelectTrigger id="job-mode" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="fresh">{t('mode.fresh')}</SelectItem>
                    <SelectItem value="exec">{t('mode.exec')}</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
            <FieldDescription>{t(`dialog.modeHint.${mode}`)}</FieldDescription>
          </Field>

          <CronScheduleFields<JobFormValues>
            idPrefix="job-schedule"
            control={control}
            register={register}
            formState={formState}
            frequency={frequency}
          />

          <Field>
            <FieldLabel htmlFor="job-timezone">
              {t('dialog.timezone')}
            </FieldLabel>
            <Input id="job-timezone" {...register('timezone')} />
            <FieldDescription>{t('dialog.timezoneHint')}</FieldDescription>
            <FieldError
              errors={[
                formState.errors.timezone && {
                  message: t('dialog.timezoneRequired'),
                },
              ]}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="job-concurrency">
              {t('dialog.concurrencyLabel')}
            </FieldLabel>
            <Controller
              control={control}
              name="concurrencyPolicy"
              render={({ field }) => (
                <Select<ScheduledTaskConcurrencyPolicy>
                  value={field.value}
                  onValueChange={(value) => {
                    field.onChange(value ?? 'forbid')
                  }}
                >
                  <SelectTrigger id="job-concurrency" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="allow">
                      {t('concurrency.allow')}
                    </SelectItem>
                    <SelectItem value="forbid">
                      {t('concurrency.forbid')}
                    </SelectItem>
                    <SelectItem value="replace">
                      {t('concurrency.replace')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
            <FieldDescription>
              {t(`concurrencyHint.${watch('concurrencyPolicy')}`)}
            </FieldDescription>
          </Field>

          <Collapsible>
            <CollapsibleTrigger
              render={<Button type="button" variant="ghost" size="sm" />}
            >
              {t('dialog.advanced')}
            </CollapsibleTrigger>
            <CollapsiblePanel className="mt-3 space-y-4">
              {numberField(
                'job-timeout',
                'timeoutSeconds',
                t('dialog.timeout'),
                t('dialog.timeoutHint'),
              )}
              {numberField(
                'job-retries',
                'maxRetries',
                t('dialog.retries'),
                t('dialog.retriesHint'),
              )}
              {numberField(
                'job-backoff',
                'retryBackoffSeconds',
                t('dialog.backoff'),
              )}
              <Field>
                <FieldLabel htmlFor="job-catch-up">
                  {t('dialog.catchUpLabel')}
                </FieldLabel>
                <Controller
                  control={control}
                  name="catchUp"
                  render={({ field }) => (
                    <Select<ScheduledTaskCatchUp>
                      value={field.value}
                      onValueChange={(value) => {
                        field.onChange(value ?? 'skip')
                      }}
                    >
                      <SelectTrigger id="job-catch-up" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="skip">
                          {t('dialog.catchUp.skip')}
                        </SelectItem>
                        <SelectItem value="run_once">
                          {t('dialog.catchUp.run_once')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
              </Field>
              {catchUp === 'run_once'
                ? numberField(
                    'job-lookback',
                    'catchUpLookbackSeconds',
                    t('dialog.lookback'),
                  )
                : null}
              {mode === 'fresh'
                ? numberField(
                    'job-memory',
                    'memoryLimitMb',
                    t('dialog.memory'),
                    t('dialog.memoryHint'),
                  )
                : null}
              {mode === 'fresh'
                ? numberField('job-cpu', 'cpuLimitMillis', t('dialog.cpu'))
                : null}
              {numberField(
                'job-warn',
                'warnAfterSeconds',
                t('dialog.warnAfter'),
                t('dialog.warnAfterHint'),
              )}
            </CollapsiblePanel>
          </Collapsible>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="job-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="job-enabled">{t('dialog.enabled')}</FieldLabel>
          </Field>

          {mutation.isError ? (
            <p className="text-sm text-destructive">{mutation.error.message}</p>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending
                ? isEdit
                  ? t('dialog.saving')
                  : t('dialog.creating')
                : isEdit
                  ? t('dialog.save')
                  : t('dialog.create')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
