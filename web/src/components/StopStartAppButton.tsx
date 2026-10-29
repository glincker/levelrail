import { PauseGlassIcon, PlayGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useStartApp, useStopApp } from '../queries/apps'
import { useAppSleepActions } from '../queries/appSleep'

// One app's stop/start toggle, split out of routes/apps/$name.tsx's
// header the same way RestartAppButton is. Unlike delete, stop is not
// destructive (the desired service row, image, env, and domains are
// untouched, only running containers go away), so this is a plain
// one-click button, not a confirm dialog, the same reasoning
// RestartAppButton's own doc comment gives.
export function StopStartAppButton({
  name,
  suspended,
  asleep = false,
}: {
  name: string
  suspended: boolean
  asleep?: boolean
}) {
  const stopApp = useStopApp()
  const startApp = useStartApp()
  const { wake } = useAppSleepActions(name)

  if (asleep) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={wake.isPending}
        onClick={() => {
          wake.mutate(undefined, {
            onSuccess: () => {
              toast.add({ title: `Waking "${name}".`, type: 'success' })
            },
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          })
        }}
      >
        <PlayGlassIcon className="size-5" aria-hidden="true" />
        {wake.isPending ? 'Waking...' : 'Wake now'}
      </Button>
    )
  }

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
        <PlayGlassIcon className="size-5" aria-hidden="true" />
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
      <PauseGlassIcon className="size-5" aria-hidden="true" />
      {stopApp.isPending ? 'Stopping...' : 'Stop'}
    </Button>
  )
}
