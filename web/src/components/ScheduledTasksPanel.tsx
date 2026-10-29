import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { CountdownGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
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

function jobTarget(task: ScheduledTask): string {
  if (task.mode === 'http' && task.http) {
    return `${task.http.method ?? 'POST'} ${task.http.path}${task.http.background ? ' (background)' : ''}`
  }
  return task.command.join(' ')
}

function jobLabel(task: ScheduledTask): string {
  return task.name || jobTarget(task)
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

// ScheduledTasksPanel is the app's Jobs tab: the job list with its
// schedule, mode and live state, then the app-wide run history.
export function ScheduledTasksPanel({ appName }: { appName: string }) {
  const { t } = useTranslation('jobs')
  const { data, isLoading, error } = useScheduledTasks(appName)
  const tasks = data ?? []
  const columns = useMemo<ResponsiveColumn<ScheduledTask>[]>(
    () => [
      {
        id: 'job',
        header: t('columns.job'),
        primary: true,
        cell: (task) => (
          <div className="flex flex-col gap-0.5">
            <span className="flex flex-wrap items-center gap-1.5 text-sm font-medium text-foreground">
              {task.name || t('unnamed')}
              {task.source === 'spec' ? (
                <Badge variant="muted" title={t('specManagedHint')}>
                  {t('specManaged')}
                </Badge>
              ) : null}
            </span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {jobTarget(task)}
            </span>
          </div>
        ),
      },
      {
        id: 'lastRun',
        header: t('columns.lastRun'),
        cell: (task) => <LastRunCell task={task} />,
      },
      {
        id: 'enabled',
        header: t('columns.enabled'),
        cell: (task) => <EnabledToggle appName={appName} task={task} />,
      },
      {
        id: 'schedule',
        header: t('columns.schedule'),
        priority: 2,
        cell: (task) => (
          <span className="font-mono text-xs text-muted-foreground">
            {task.schedule}
            {task.timezone && task.timezone !== 'UTC' ? (
              <span className="block font-sans">{task.timezone}</span>
            ) : null}
          </span>
        ),
      },
      {
        id: 'nextRun',
        header: t('columns.nextRun'),
        priority: 2,
        cell: (task) => (
          <span className="text-xs text-muted-foreground">
            {task.enabled && task.next_run_at
              ? new Date(task.next_run_at).toLocaleString()
              : '-'}
          </span>
        ),
      },
      {
        id: 'mode',
        header: t('columns.mode'),
        priority: 3,
        cell: (task) => (
          <span className="text-xs text-muted-foreground">
            {t(`mode.${task.mode ?? 'exec'}`)}
          </span>
        ),
      },
    ],
    [appName, t],
  )

  return (
    <div className="space-y-4">
      <section className="rounded-lg border border-border p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
              <CountdownGlassIcon
                className="size-4.5 text-muted-foreground"
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
              icon={<CountdownGlassIcon className="size-5.5" />}
              title={t('empty.title')}
              description={t('empty.description')}
              action={<JobDialog appName={appName} />}
            />
          ) : (
            <ResponsiveTable
              rows={tasks}
              columns={columns}
              getRowId={(task) => task.id}
              ariaLabel={t('title')}
              renderActions={(task) => (
                <span className="flex flex-wrap justify-end gap-2">
                  <RunScheduledTaskButton appName={appName} task={task} />
                  <JobDialog appName={appName} task={task} />
                  <DeleteScheduledTaskDialog appName={appName} task={task} />
                </span>
              )}
            />
          )}
        </div>
      </section>

      {tasks.length > 0 ? <JobRunsPanel appName={appName} /> : null}
    </div>
  )
}
