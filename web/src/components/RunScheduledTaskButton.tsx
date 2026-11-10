import { useTranslation } from 'react-i18next'
import { PlayGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRunScheduledTaskNow } from '../queries/scheduledTasks'
import { jobErrorMessage } from '../lib/jobAccess'
import type { ScheduledTask } from '../types/scheduledTasks'
import { CopyAsCommand } from './CopyAsCommand'

// One job's "run now" trigger: a one-click action like a manual restart,
// since it only runs the operator's own already-saved command on demand.
export function RunScheduledTaskButton({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const { t } = useTranslation('jobs')
  const runNow = useRunScheduledTaskNow(appName)

  return (
    <span className="inline-flex items-center gap-1">
      <Button
        variant="outline"
        size="sm"
        disabled={runNow.isPending || Boolean(task.running_since)}
        onClick={() => {
          runNow.mutate(task.id, {
            onSuccess: (started) => {
              toast.add({
                title: started.run_id
                  ? t('run.startedWithId', { id: started.run_id })
                  : t('run.started'),
                type: 'success',
              })
            },
            onError: (error) => {
              toast.add({
                title: jobErrorMessage(error, t('accessRule')),
                type: 'error',
              })
            },
          })
        }}
      >
        <PlayGlassIcon className="size-5" aria-hidden="true" />
        {runNow.isPending
          ? t('run.starting')
          : task.running_since
            ? t('run.running')
            : t('run.now')}
      </Button>
      <CopyAsCommand
        subject={task.name ?? task.id}
        actions={[{ kind: 'job.run', app: appName, jobId: task.id }]}
      />
    </span>
  )
}
