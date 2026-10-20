import { PauseIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useStartProject, useStopProject } from '../queries/projects'

// One project's pause/resume actions: stop or start every app and
// database filed under it in one call (POST /api/v1/projects/{id}/stop
// and .../start). Unlike StopStartAppButton's single toggle, this always
// shows both actions rather than one based on current state: a
// database's own suspended flag isn't in GET /api/v1/databases' response
// yet (only an app's is), so there is no reliable "is everything in this
// project currently stopped" signal to toggle on. Not a confirm dialog,
// the same reasoning StopStartAppButton gives: stopping is not
// destructive, every desired-state row survives untouched.
export function PauseResumeProjectButton({
  id,
  name,
}: {
  id: string
  name: string
}) {
  const stopProject = useStopProject()
  const startProject = useStartProject()

  return (
    <div className="flex items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={stopProject.isPending}
        onClick={() => {
          stopProject.mutate(id, {
            onSuccess: (result) => {
              toast.add({
                title: `Stopping "${name}": ${result.succeeded_apps.length} app(s), ${result.succeeded_databases.length} database(s).`,
                type: 'success',
              })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        }}
      >
        <PauseIcon className="size-3.5" aria-hidden="true" />
        {stopProject.isPending ? 'Stopping...' : 'Stop project'}
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={startProject.isPending}
        onClick={() => {
          startProject.mutate(id, {
            onSuccess: (result) => {
              toast.add({
                title: `Starting "${name}": ${result.succeeded_apps.length} app(s), ${result.succeeded_databases.length} database(s).`,
                type: 'success',
              })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        }}
      >
        <PlayIcon className="size-3.5" aria-hidden="true" />
        {startProject.isPending ? 'Starting...' : 'Start project'}
      </Button>
    </div>
  )
}
