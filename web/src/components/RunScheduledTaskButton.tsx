import { PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRunScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

// A single "run now" action: POST .../scheduled-tasks/{id}/run returns
// as soon as the attempt is recorded and under way (202), not once the
// command finishes, so this only ever confirms the trigger succeeded,
// pointing at the History dialog for the real outcome.
export function RunScheduledTaskButton({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const runTask = useRunScheduledTask(appName)

  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={runTask.isPending}
      onClick={() => {
        runTask.mutate(task.id, {
          onSuccess: () => {
            toast.add({
              title: `Run started for "${task.name}".`,
              description: 'Check History for the result once it finishes.',
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `Failed to start run for "${task.name}".`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlayIcon className="size-3.5" aria-hidden="true" />
      {runTask.isPending ? 'Starting...' : 'Run now'}
    </Button>
  )
}
