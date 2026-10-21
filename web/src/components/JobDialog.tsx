import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  PencilSimpleIcon,
  PlusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
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
} from '../types/scheduledTasks'
import {
  DEFAULT_JOB_VALUES,
  jobSchema,
  toFormValues,
  toRequest,
  type JobFormValues,
} from '../lib/jobForm'
import { toCron } from '../lib/cronSchedule'
import { CronScheduleFields } from './CronScheduleFields'
import { CronPreview } from './CronPreview'
import { JobHttpFields } from './JobHttpFields'

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

  const defaultValues = task ? toFormValues(task) : DEFAULT_JOB_VALUES
  const { control, register, handleSubmit, formState, reset, watch } =
    useForm<JobFormValues>({
      resolver: zodResolver(jobSchema),
      defaultValues,
    })

  const frequency = watch('frequency')
  const mode = watch('mode')
  const catchUp = watch('catchUp')
  const timezone = watch('timezone')
  const cron = toCron({
    frequency,
    time: watch('time'),
    weekday: watch('weekday'),
    customCron: watch('customCron'),
  })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req = toRequest(values)
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
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetTrigger
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
      <SheetContent className="overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>
            {isEdit ? t('dialog.editTitle') : t('dialog.createTitle')}
          </SheetTitle>
          <SheetDescription>{t('description')}</SheetDescription>
        </SheetHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4 px-4 pb-4"
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
                    <SelectItem value="http">{t('mode.http')}</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
            <FieldDescription>{t(`dialog.modeHint.${mode}`)}</FieldDescription>
          </Field>

          {mode === 'http' ? (
            <JobHttpFields
              control={control}
              register={register}
              formState={formState}
            />
          ) : (
            <Field>
              <FieldLabel htmlFor="job-command">
                {t('dialog.command')}
              </FieldLabel>
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
          )}

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

          <CronPreview cron={cron} timezone={timezone} />

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
                    <SelectItem value="queue">
                      {t('concurrency.queue')}
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

          <SheetFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending
                ? isEdit
                  ? t('dialog.saving')
                  : t('dialog.creating')
                : isEdit
                  ? t('dialog.save')
                  : t('dialog.create')}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
