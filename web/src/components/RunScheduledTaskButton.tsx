import { PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRunScheduledTaskNow } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

// One task's "run now" trigger, mirroring RestartAppButton's own
// one-click (not confirm-dialog) shape: this doesn't destroy anything of
// this platform's own state, it runs the operator's own already-saved
// command on demand instead of waiting for its schedule, the same class
// of action as a manual restart.
export function RunScheduledTaskButton({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const runNow = useRunScheduledTaskNow(appName)

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={runNow.isPending}
      onClick={() => {
        runNow.mutate(task.id, {
          onSuccess: () => {
            toast.add({ title: 'Task triggered.', type: 'success' })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <PlayIcon className="size-3.5" aria-hidden="true" />
      {runNow.isPending ? 'Running...' : 'Run now'}
    </Button>
  )
}
