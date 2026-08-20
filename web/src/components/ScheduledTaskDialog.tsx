import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PencilSimpleIcon, PlusCircleIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateScheduledTask, useUpdateScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask, ScheduledTaskRequest } from '../types/scheduledTasks'

// Sanity-check for a standard 5-field cron expression ("minute hour
// day-of-month month day-of-week"), the exact grammar internal/cronexpr
// parses server-side (cronexpr.Parse). This is a shape check, not a real
// parser: it catches an obviously wrong field count or character before
// a round trip, the same "catch the obvious case, let the server's own
// error surface whatever this lets through" reasoning
// CreateAlertRuleDialog's own GO_DURATION_REGEX comment establishes for
// a different field.
const CRON_FIELD_REGEX = /^[0-9*/,-]+$/

function looksLikeCronExpression(value: string): boolean {
  const fields = value.trim().split(/\s+/)
  return fields.length === 5 && fields.every((field) => CRON_FIELD_REGEX.test(field))
}

const scheduledTaskSchema = z.object({
  command: z.string().trim().min(1, 'Command is required'),
  schedule: z
    .string()
    .trim()
    .min(1, 'Schedule is required')
    .refine(looksLikeCronExpression, {
      message: 'Must be a 5-field cron expression, e.g. "0 3 * * *"',
    }),
  enabled: z.boolean(),
})

type ScheduledTaskFormValues = z.infer<typeof scheduledTaskSchema>

const DEFAULT_VALUES: ScheduledTaskFormValues = {
  command: '',
  schedule: '0 3 * * *',
  enabled: true,
}

// commandToDisplay/displayToCommand round-trip the backend's argv-array
// Command through a single shell-command-line input: the common case an
// operator types ("rm -rf /tmp/cache/*") becomes ["sh", "-c", "rm -rf
// /tmp/cache/*"], never shell-interpreted by this platform itself (the
// same argv-array contract internal/api/exec.go's own execRequest
// establishes), just given a friendlier editing surface than a raw JSON
// array field. A task whose command wasn't created through this exact
// ["sh", "-c", ...] shape (e.g. authored directly through the API)
// still round-trips readably: this falls back to space-joining the argv
// elements, which won't necessarily reproduce the original array on
// re-save if any argument itself contained a space, but is still an
// honest, editable display of what will actually run.
function commandToDisplay(command: string[]): string {
  if (command.length === 3 && command[0] === 'sh' && command[1] === '-c') {
    return command[2] ?? ''
  }
  return command.join(' ')
}

function displayToCommand(value: string): string[] {
  return ['sh', '-c', value.trim()]
}

export function ScheduledTaskDialog({
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

  const defaultValues: ScheduledTaskFormValues = task
    ? { command: commandToDisplay(task.command), schedule: task.schedule, enabled: task.enabled }
    : DEFAULT_VALUES

  const { control, register, handleSubmit, formState, reset } = useForm<ScheduledTaskFormValues>({
    resolver: zodResolver(scheduledTaskSchema),
    defaultValues,
  })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req: ScheduledTaskRequest = {
      command: displayToCommand(values.command),
      schedule: values.schedule.trim(),
      enabled: values.enabled,
    }
    const onSuccess = () => {
      handleOpenChange(false)
      toast.add({
        title: isEdit ? 'Scheduled task updated.' : 'Scheduled task created.',
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
            <Button variant="outline" size="sm">
              <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
              Edit
            </Button>
          ) : (
            <Button size="sm">
              <PlusCircleIcon className="size-3.5" aria-hidden="true" />
              Create task
            </Button>
          )
        }
      />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            {isEdit ? (
              <PencilSimpleIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            ) : (
              <PlusCircleIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            )}
            {isEdit ? 'Edit scheduled task' : 'Create scheduled task'}
          </DialogTitle>
          <DialogDescription>
            Runs a command inside this app&apos;s currently running container on
            a cron schedule, e.g. a nightly cleanup script or a periodic
            cache-warm job.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="task-command">Command</FieldLabel>
            <Input
              id="task-command"
              placeholder="e.g. rm -rf /tmp/cache/*"
              {...register('command')}
            />
            <FieldDescription>
              Run through a shell (<code>sh -c</code>) inside the container,
              never interpreted by Levelrail itself.
            </FieldDescription>
            <FieldError errors={[formState.errors.command]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="task-schedule">Schedule</FieldLabel>
            <Input
              id="task-schedule"
              placeholder="0 3 * * *"
              {...register('schedule')}
            />
            <FieldDescription>
              Standard 5-field cron: minute hour day-of-month month
              day-of-week.
            </FieldDescription>
            <FieldError errors={[formState.errors.schedule]} />
          </Field>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="task-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="task-enabled">Enabled</FieldLabel>
          </Field>

          {mutation.isError ? (
            <p className="text-sm text-destructive">{mutation.error.message}</p>
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
