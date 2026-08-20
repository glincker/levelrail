import { ClockCountdownIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { EmptyState } from '@/components/ui/empty-state'
import { ScheduledTaskDialog } from './ScheduledTaskDialog'
import { DeleteScheduledTaskDialog } from './DeleteScheduledTaskDialog'
import { RunScheduledTaskButton } from './RunScheduledTaskButton'
import { useScheduledTasks, useUpdateScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask, ScheduledTaskRunStatus } from '../types/scheduledTasks'

const STATUS_LABEL: Record<ScheduledTaskRunStatus, string> = {
  success: 'Success',
  failed: 'Failed',
  timeout: 'Timed out',
  container_not_running: 'Container not running',
}

// success -> the green "everything's fine" badge every other status
// badge in this dashboard uses for a healthy outcome (ConditionsPanel's
// own STATUS_BADGE_VARIANT precedent); failed/timeout are both real
// problems an operator needs to notice, so both get destructive/warning
// rather than one flat "bad" bucket; container_not_running is neither a
// command failure nor a platform bug, just "there was nothing to run
// against", so it gets a neutral muted badge instead of implying
// something broke.
const STATUS_BADGE_VARIANT: Record<
  ScheduledTaskRunStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  success: 'success',
  failed: 'destructive',
  timeout: 'warning',
  container_not_running: 'muted',
}

function LastRunCell({ task }: { task: ScheduledTask }) {
  if (!task.last_run_at || !task.last_run_status) {
    return <span className="text-muted-foreground">Never run</span>
  }
  return (
    <div className="flex max-w-[20rem] flex-col gap-1">
      <Badge variant={STATUS_BADGE_VARIANT[task.last_run_status]}>
        {STATUS_LABEL[task.last_run_status]}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {new Date(task.last_run_at).toLocaleString()}
      </span>
      {task.last_run_output ? (
        <details className="text-xs">
          <summary className="cursor-pointer text-muted-foreground underline underline-offset-2">
            Output
          </summary>
          <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-2 font-mono text-[11px] text-foreground">
            {task.last_run_output}
          </pre>
        </details>
      ) : null}
    </div>
  )
}

// EnabledToggle flips a task's Enabled flag in place through the same
// PUT .../scheduled-tasks/{id} the edit dialog uses, resubmitting its
// current Command/Schedule unchanged: there is no separate PATCH-just-
// enabled route, matching UpdateScheduledTask's own "Command, Schedule,
// and Enabled are updated together" shape (internal/store/
// scheduled_task.go).
function EnabledToggle({ appName, task }: { appName: string; task: ScheduledTask }) {
  const updateTask = useUpdateScheduledTask(appName)

  return (
    <Switch
      checked={task.enabled}
      disabled={updateTask.isPending}
      onCheckedChange={(checked) => {
        updateTask.mutate(
          {
            id: task.id,
            req: { command: task.command, schedule: task.schedule, enabled: checked },
          },
          {
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          },
        )
      }}
    />
  )
}

function TaskRow({ appName, task }: { appName: string; task: ScheduledTask }) {
  return (
    <TableRow>
      <TableCell className="max-w-[24rem] truncate font-mono text-xs text-foreground">
        {task.command.join(' ')}
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {task.schedule}
      </TableCell>
      <TableCell>
        <LastRunCell task={task} />
      </TableCell>
      <TableCell>
        <EnabledToggle appName={appName} task={task} />
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-2">
          <RunScheduledTaskButton appName={appName} task={task} />
          <ScheduledTaskDialog appName={appName} task={task} />
          <DeleteScheduledTaskDialog appName={appName} task={task} />
        </div>
      </TableCell>
    </TableRow>
  )
}

export function ScheduledTasksPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useScheduledTasks(appName)
  const tasks = data ?? []

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <ClockCountdownIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Scheduled tasks
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Runs a command inside this app&apos;s currently running container
            on a cron schedule, e.g. a nightly cleanup script or a periodic
            cache-warm job.
          </p>
        </div>
        <ScheduledTaskDialog appName={appName} />
      </div>

      <div className="mt-3">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : tasks.length === 0 ? (
          <EmptyState
            icon={<ClockCountdownIcon className="size-5" />}
            title="No scheduled tasks yet"
            description="Run a command inside this app's container on a cron schedule, e.g. a nightly cleanup script or a periodic cache-warm job."
            action={<ScheduledTaskDialog appName={appName} />}
          />
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Command</TableHead>
                  <TableHead>Schedule</TableHead>
                  <TableHead>Last run</TableHead>
                  <TableHead>Enabled</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <TaskRow key={task.id} appName={appName} task={task} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </section>
  )
}
