import { useTranslation } from 'react-i18next'
import { PlayGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRunScheduledTaskNow } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

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
    <Button
      variant="outline"
      size="sm"
      disabled={runNow.isPending}
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
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <PlayGlassIcon className="size-4" aria-hidden="true" />
      {runNow.isPending ? t('run.starting') : t('run.now')}
    </Button>
  )
}
