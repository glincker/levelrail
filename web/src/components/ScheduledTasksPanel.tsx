import { useTranslation } from 'react-i18next'
import { ClockCountdownIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { JobDialog } from './JobDialog'
import { JobRunsPanel } from './JobRunsPanel'
import { JobStatusBadge } from './JobStatusBadge'
import { DeleteScheduledTaskDialog } from './DeleteScheduledTaskDialog'
import { RunScheduledTaskButton } from './RunScheduledTaskButton'
import {
  useScheduledTasks,
  useUpdateScheduledTask,
} from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

function jobLabel(task: ScheduledTask): string {
  return task.name || task.command.join(' ')
}

function LastRunCell({ task }: { task: ScheduledTask }) {
  const { t } = useTranslation('jobs')
  if (task.running_since) {
    return <JobStatusBadge status="running" />
  }
  if (!task.last_run_at || !task.last_run_status) {
    return <span className="text-muted-foreground">{t('neverRun')}</span>
  }
  return (
    <div className="flex flex-col gap-1">
      <JobStatusBadge status={task.last_run_status} />
      <span className="text-xs text-muted-foreground">
        {new Date(task.last_run_at).toLocaleString()}
      </span>
      {task.consecutive_failures > 0 ? (
        <span className="text-xs text-destructive">
          {t('failedInARow', { count: task.consecutive_failures })}
        </span>
      ) : null}
      {(task.consecutive_missed ?? 0) > 0 ? (
        <span className="text-xs text-warning">
          {t('missedInARow', { count: task.consecutive_missed })}
        </span>
      ) : null}
    </div>
  )
}

// EnabledToggle flips Enabled through the same PUT the edit dialog uses;
// the server merges the body over the stored job, so only enabled is sent.
function EnabledToggle({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const { t } = useTranslation('jobs')
  const updateTask = useUpdateScheduledTask(appName)

  return (
    <Switch
      aria-label={t('toggle.label', { name: jobLabel(task) })}
      checked={task.enabled}
      disabled={updateTask.isPending}
      onCheckedChange={(checked) => {
        updateTask.mutate(
          {
            id: task.id,
            req: {
              command: task.command,
              schedule: task.schedule,
              enabled: checked,
            },
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

function JobRow({ appName, task }: { appName: string; task: ScheduledTask }) {
  const { t } = useTranslation('jobs')
  return (
    <TableRow>
      <TableCell className="max-w-[24rem]">
        <div className="flex flex-col gap-0.5">
          <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
            {task.name || t('unnamed')}
            {task.source === 'spec' ? (
              <Badge variant="muted" title={t('specManagedHint')}>
                {t('specManaged')}
              </Badge>
            ) : null}
          </span>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {task.command.join(' ')}
          </span>
        </div>
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {task.schedule}
        {task.timezone && task.timezone !== 'UTC' ? (
          <div className="font-sans">{task.timezone}</div>
        ) : null}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {t(`mode.${task.mode ?? 'exec'}`)}
      </TableCell>
      <TableCell>
        <LastRunCell task={task} />
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {task.enabled && task.next_run_at
          ? new Date(task.next_run_at).toLocaleString()
          : '-'}
      </TableCell>
      <TableCell>
        <EnabledToggle appName={appName} task={task} />
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-2">
          <RunScheduledTaskButton appName={appName} task={task} />
          <JobDialog appName={appName} task={task} />
          <DeleteScheduledTaskDialog appName={appName} task={task} />
        </div>
      </TableCell>
    </TableRow>
  )
}

// ScheduledTasksPanel is the app's Jobs tab: the job list with its
// schedule, mode and live state, then the app-wide run history.
export function ScheduledTasksPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('jobs')
  const { data, isLoading, error } = useScheduledTasks(appName)
  const tasks = data ?? []

  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-border p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
              <ClockCountdownIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
              {t('title')}
            </h2>
            <p className="mt-1 max-w-2xl text-xs text-muted-foreground">
              {t('description')}
            </p>
          </div>
          <JobDialog appName={appName} />
        </div>

        <div className="mt-3">
          {isLoading ? (
            <TableSkeleton columnCount={7} rowCount={3} />
          ) : error ? (
            <p className="text-sm text-destructive">{error.message}</p>
          ) : tasks.length === 0 ? (
            <EmptyState
              icon={<ClockCountdownIcon className="size-5" />}
              title={t('empty.title')}
              description={t('empty.description')}
              action={<JobDialog appName={appName} />}
            />
          ) : (
            <div className="rounded-lg border border-border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('columns.job')}</TableHead>
                    <TableHead>{t('columns.schedule')}</TableHead>
                    <TableHead>{t('columns.mode')}</TableHead>
                    <TableHead>{t('columns.lastRun')}</TableHead>
                    <TableHead>{t('columns.nextRun')}</TableHead>
                    <TableHead>{t('columns.enabled')}</TableHead>
                    <TableHead className="text-right">
                      {t('columns.actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {tasks.map((task) => (
                    <JobRow key={task.id} appName={appName} task={task} />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </div>
      </section>

      {tasks.length > 0 ? <JobRunsPanel appName={appName} /> : null}
    </div>
  )
}
