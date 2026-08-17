import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { ClockIcon, PencilSimpleIcon, PlusCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateScheduledTask, useUpdateScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

// A loose sanity check, not a real cron parser: five whitespace-separated
// fields. The real validation is internal/cronexpr.Parse, run
// server-side on every create/update call; this only catches an
// obviously-wrong submission (a Unix crontab line accidentally missing a
// field) before a round trip, the same "client fast-feedback only, the
// server is the real authority" reasoning CreateAlertRuleDialog's own
// GO_DURATION_REGEX comment gives.
const CRON_FIELD_COUNT_REGEX = /^\S+\s+\S+\s+\S+\s+\S+\s+\S+$/

// timeoutSeconds stays a plain trimmed string, not z.coerce.number(): an
// empty string here means "use the server's own default", which a
// coerced number field can't represent (Number('') is 0, a real,
// different value), the same reasoning CreateAppFields.tsx's own
// replicas field comment gives. Parsed into a real optional number in
// onSubmit below, not in this schema.
const scheduledTaskSchema = z.object({
  name: z.string().trim().min(1, 'Name is required'),
  command: z.string().trim().min(1, 'Command is required'),
  schedule: z
    .string()
    .trim()
    .min(1, 'Schedule is required')
    .regex(CRON_FIELD_COUNT_REGEX, 'Must be a 5-field cron expression, e.g. "*/5 * * * *"'),
  enabled: z.boolean(),
  timeoutSeconds: z
    .string()
    .trim()
    .regex(/^\d*$/, 'Must be a non-negative whole number'),
})

type ScheduledTaskFormValues = z.infer<typeof scheduledTaskSchema>

function defaultValuesFor(task?: ScheduledTask): ScheduledTaskFormValues {
  return {
    name: task?.name ?? '',
    command: task?.command ?? '',
    schedule: task?.schedule ?? '',
    enabled: task?.enabled ?? true,
    timeoutSeconds: task?.timeout_seconds ? String(task.timeout_seconds) : '',
  }
}

// ScheduledTaskFormDialog handles both create (no task prop, "Create
// task" trigger) and edit (task prop supplied, an icon-only edit
// trigger) through the same form: PUT is a full replace of every
// editable field server-side (internal/api's own scheduledTaskRequest
// doc comment), so both modes send the identical request shape, only
// the mutation and the trigger/title differ.
export function ScheduledTaskFormDialog({
  appName,
  task,
}: {
  appName: string
  task?: ScheduledTask
}) {
  const [open, setOpen] = useState(false)
  const isEdit = task !== undefined
  const createTask = useCreateScheduledTask(appName)
  const updateTask = useUpdateScheduledTask(appName)
  const mutation = isEdit ? updateTask : createTask

  const { control, register, handleSubmit, formState, reset } =
    useForm<ScheduledTaskFormValues>({
      resolver: zodResolver(scheduledTaskSchema),
      defaultValues: defaultValuesFor(task),
    })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValuesFor(task))
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req = {
      name: values.name.trim(),
      command: values.command.trim(),
      schedule: values.schedule.trim(),
      enabled: values.enabled,
      ...(values.timeoutSeconds
        ? { timeout_seconds: Number(values.timeoutSeconds) }
        : {}),
    }

    const onSuccess = () => {
      handleOpenChange(false)
      toast.add({
        title: isEdit
          ? `Scheduled task "${values.name.trim()}" updated.`
          : `Scheduled task "${values.name.trim()}" created.`,
        type: 'success',
      })
    }

    if (isEdit) {
      updateTask.mutate({ id: task.id, req }, { onSuccess })
    } else {
      createTask.mutate(req, { onSuccess })
    }
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          isEdit ? (
            <Button variant="outline" size="sm" aria-label="Edit scheduled task" />
          ) : (
            <Button size="sm" />
          )
        }
      >
        {isEdit ? (
          <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <>
            <PlusCircleIcon className="size-4" aria-hidden="true" />
            Create task
          </>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ClockIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            {isEdit ? 'Edit scheduled task' : 'Create scheduled task'}
          </DialogTitle>
          <DialogDescription>
            Runs a command inside this app&apos;s container on a cron
            schedule, executed as <code>sh -c COMMAND</code>.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="scheduled-task-name">Name</FieldLabel>
            <Input
              id="scheduled-task-name"
              placeholder="e.g. laravel-scheduler"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="scheduled-task-command">Command</FieldLabel>
            <Input
              id="scheduled-task-command"
              className="font-mono"
              placeholder="php artisan schedule:run"
              {...register('command')}
            />
            <FieldError errors={[formState.errors.command]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="scheduled-task-schedule">Schedule</FieldLabel>
            <Input
              id="scheduled-task-schedule"
              className="font-mono"
              placeholder="*/5 * * * *"
              {...register('schedule')}
            />
            <FieldDescription>Standard 5-field cron expression.</FieldDescription>
            <FieldError errors={[formState.errors.schedule]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="scheduled-task-timeout">
              Timeout (seconds){' '}
              <span className="text-muted-foreground">(optional)</span>
            </FieldLabel>
            <Input
              id="scheduled-task-timeout"
              type="number"
              min={0}
              placeholder="e.g. 300"
              {...register('timeoutSeconds')}
            />
            <FieldError errors={[formState.errors.timeoutSeconds]} />
          </Field>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="scheduled-task-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="scheduled-task-enabled">Enabled</FieldLabel>
          </Field>

          {mutation.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
              <AlertDescription>{mutation.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending
                ? isEdit
                  ? 'Saving...'
                  : 'Creating...'
                : isEdit
                  ? 'Save changes'
                  : 'Create task'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
