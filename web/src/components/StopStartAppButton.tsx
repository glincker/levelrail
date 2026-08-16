import { PauseIcon, PlayIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useStartApp, useStopApp } from '../queries/apps'

// One app's stop/start toggle, split out of routes/apps/$name.tsx's
// header the same way RestartAppButton is. Unlike delete, stop is not
// destructive (the desired service row, image, env, and domains are
// untouched, only running containers go away), so this is a plain
// one-click button, not a confirm dialog, the same reasoning
// RestartAppButton's own doc comment gives.
export function StopStartAppButton({
  name,
  suspended,
}: {
  name: string
  suspended: boolean
}) {
  const stopApp = useStopApp()
  const startApp = useStartApp()

  if (suspended) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={startApp.isPending}
        onClick={() => {
          startApp.mutate(name, {
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
        {startApp.isPending ? 'Starting...' : 'Start'}
      </Button>
    )
  }

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={stopApp.isPending}
      onClick={() => {
        stopApp.mutate(name, {
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
      {stopApp.isPending ? 'Stopping...' : 'Stop'}
    </Button>
  )
}
