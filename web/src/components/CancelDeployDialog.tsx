import { ProhibitIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useCancelDeploy } from '../queries/deployControl'

// One deploy attempt's cancel confirm, split out of DeployAttemptsList's
// ActionMenu the same way DeleteAppDialog is split out of the app header.
// POST .../cancel stops a running build or drops a queued one; there is
// no undo once it fires, so this is a confirm dialog, not the one-click
// danger-tone menu item it used to be.
export function CancelDeployDialog({
  appName,
  deployId,
  running,
  open,
  onOpenChange,
}: {
  appName: string
  deployId: string
  running: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const cancelDeploy = useCancelDeploy(appName)

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) {
      cancelDeploy.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Cancel this deploy?
          </DialogTitle>
          <DialogDescription>
            {running
              ? 'Stops the build in progress. The release currently serving traffic is untouched.'
              : 'Removes this deploy from the queue.'}
          </DialogDescription>
        </DialogHeader>
        {cancelDeploy.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{cancelDeploy.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Keep it running
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={cancelDeploy.isPending}
            onClick={() => {
              cancelDeploy.mutate(deployId, {
                onSuccess: () => {
                  handleOpenChange(false)
                  toast.add({ title: 'Deploy canceled.', type: 'success' })
                },
              })
            }}
          >
            <ProhibitIcon className="size-3.5" aria-hidden="true" />
            {cancelDeploy.isPending ? 'Canceling...' : 'Cancel deploy'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
