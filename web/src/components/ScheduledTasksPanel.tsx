import { ClockIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { ScheduledTaskFormDialog } from './ScheduledTaskFormDialog'
import { DeleteScheduledTaskDialog } from './DeleteScheduledTaskDialog'
import { RunScheduledTaskButton } from './RunScheduledTaskButton'
import { ScheduledTaskRunsDialog } from './ScheduledTaskRunsDialog'
import { useScheduledTasks } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

function ScheduledTaskRow({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  return (
    <TableRow>
      <TableCell className="font-medium text-foreground">
        {task.name}
        {!task.enabled ? (
          <Badge variant="muted" className="ml-2">
            Disabled
          </Badge>
        ) : null}
      </TableCell>
      <TableCell className="max-w-[20rem] truncate font-mono text-xs text-muted-foreground">
        {task.command}
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {task.schedule}
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-2">
          <RunScheduledTaskButton appName={appName} task={task} />
          <ScheduledTaskRunsDialog appName={appName} task={task} />
          <ScheduledTaskFormDialog appName={appName} task={task} />
          <DeleteScheduledTaskDialog appName={appName} task={task} />
        </div>
      </TableCell>
    </TableRow>
  )
}

// App detail page section for scheduled tasks (an arbitrary command run
// inside the app's container on a cron schedule), the closest gap this
// closes to Coolify's ScheduledTask.php per
// docs-local/research/coolify-feature-comparison.md §13. Same section
// shape AlertRulesPanel already establishes: a plain useQuery-backed
// table with a create trigger in its own header, not loader-primed.
export function ScheduledTasksPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useScheduledTasks(appName)
  const tasks = data ?? []

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <ClockIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            Scheduled tasks
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Arbitrary commands run inside this app&apos;s container on a cron
            schedule, e.g. a Laravel scheduler or a cleanup script.
          </p>
        </div>
        <ScheduledTaskFormDialog appName={appName} />
      </div>

      <div className="mt-3">
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No scheduled tasks yet. Create one to run a command on a cron
            schedule.
          </p>
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Command</TableHead>
                  <TableHead>Schedule</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks.map((task) => (
                  <ScheduledTaskRow key={task.id} appName={appName} task={task} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </section>
  )
}
