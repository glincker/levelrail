import { ArrowClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRestartApp } from '../queries/apps'

// One app's restart action, split out of routes/apps/$name.tsx's header
// the same way DeleteAppDialog is. Unlike delete, restart is not
// destructive or irreversible (it forces a container recreation, not a
// data loss), so this is a plain one-click button, not a confirm dialog:
// the reconciler's own blue-green cutover (this app's default strategy)
// already proves the new container healthy before the old one is
// removed, the same safety property an ordinary redeploy gets.
export function RestartAppButton({ name }: { name: string }) {
  const restartApp = useRestartApp()

  return (
    <Button
      variant="outline"
      size="sm"
      disabled={restartApp.isPending}
      onClick={() => {
        restartApp.mutate(name, {
          onSuccess: () => {
            toast.add({ title: `Restarting "${name}".`, type: 'success' })
          },
          onError: (error) => {
            toast.add({ title: error.message, type: 'error' })
          },
        })
      }}
    >
      <ArrowClockwiseIcon className="size-3.5" aria-hidden="true" />
      {restartApp.isPending ? 'Restarting...' : 'Restart'}
    </Button>
  )
}
