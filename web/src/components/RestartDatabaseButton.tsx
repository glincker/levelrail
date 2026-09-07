import { ArrowClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRestartDatabase } from '../queries/databases'

// One database's restart action, mirroring RestartAppButton's shape.
// Unlike an app restart, this briefly stops the engine process (see
// handleRestartDatabase's own doc comment for why a database container
// is stopped and started in place rather than recreated blue-green), so
// this warns about that in its confirmation toast rather than treating
// it as a zero-downtime action the way RestartAppButton's own comment
// explains apps get.
export function RestartDatabaseButton({ name }: { name: string }) {
  const restartDatabase = useRestartDatabase()

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={restartDatabase.isPending}
      onClick={() => {
        restartDatabase.mutate(name, {
          onSuccess: () => {
            toast.add({ title: `Restarted "${name}".`, type: 'success' })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <ArrowClockwiseIcon className="size-3.5" aria-hidden="true" />
      {restartDatabase.isPending ? 'Restarting...' : 'Restart'}
    </Button>
  )
}
