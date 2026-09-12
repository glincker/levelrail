import { PauseIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useStartDatabase, useStopDatabase } from '../queries/databases'

// One database's stop/start toggle, mirroring StopStartAppButton's
// shape. Unlike delete, stop is not destructive (the desired database
// row, engine, version, and data volume are untouched, only the running
// container goes away), so this is a plain one-click button, not a
// confirm dialog, the same reasoning StopStartAppButton's own doc
// comment gives.
export function StopStartDatabaseButton({
  name,
  suspended,
}: {
  name: string
  suspended: boolean
}) {
  const stopDatabase = useStopDatabase()
  const startDatabase = useStartDatabase()

  if (suspended) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={startDatabase.isPending}
        onClick={() => {
          startDatabase.mutate(name, {
            onSuccess: () => {
              toast.add({ title: `Starting "${name}".`, type: 'success' })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        }}
      >
        <PlayIcon className="size-3.5" aria-hidden="true" />
        {startDatabase.isPending ? 'Starting...' : 'Start'}
      </Button>
    )
  }

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={stopDatabase.isPending}
      onClick={() => {
        stopDatabase.mutate(name, {
          onSuccess: () => {
            toast.add({ title: `Stopping "${name}".`, type: 'success' })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <PauseIcon className="size-3.5" aria-hidden="true" />
      {stopDatabase.isPending ? 'Stopping...' : 'Stop'}
    </Button>
  )
}
