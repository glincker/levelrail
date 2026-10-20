import { ArrowClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRestartProject } from '../queries/projects'

// Bulk restart every app filed under a project, mirroring
// RestartAppButton's own plain one-click shape (not destructive or
// irreversible, so no confirm dialog): one app's restart failing
// doesn't stop the rest, so the toast reports both counts rather than
// a single pass/fail outcome.
export function RestartProjectButton({
  id,
  disabled,
}: {
  id: string
  /** True when the project has no apps to restart at all. */
  disabled?: boolean
}) {
  const restartProject = useRestartProject()

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={disabled || restartProject.isPending}
      onClick={() => {
        restartProject.mutate(id, {
          onSuccess: (result) => {
            if (result.failed && result.failed.length > 0) {
              toast.add({
                title: `Restarted ${result.restarted_count} app(s), ${result.failed.length} failed: ${result.failed.join(', ')}.`,
                type: 'error',
              })
              return
            }
            toast.add({
              title:
                result.restarted_count === 0
                  ? 'No apps to restart in this project.'
                  : `Restarting ${result.restarted_count} app(s).`,
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <ArrowClockwiseIcon className="size-3.5" aria-hidden="true" />
      {restartProject.isPending ? 'Restarting...' : 'Restart all'}
    </Button>
  )
}
