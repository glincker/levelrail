import { useState } from 'react'
import { XCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useCancelDeploy } from '../queries/builds'

// A running manual build/deploy's cancel action (POST
// .../deploys/{attemptId}/cancel, internal/api/deploy_cancel.go). A
// confirm dialog, not RestartAppButton's plain one-click button: unlike a
// restart, cancelling discards in-progress build work with no undo, the
// same destructive-and-irreversible reasoning DeleteAppDialog's own
// header comment gives.
export function CancelDeployDialog({
  appName,
  attemptId,
}: {
  appName: string
  attemptId: string
}) {
  const [open, setOpen] = useState(false)
  const cancelDeploy = useCancelDeploy(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      cancelDeploy.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <XCircleIcon className="size-3.5" data-icon="inline-start" aria-hidden="true" />
        Cancel
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Cancel this deploy?
          </DialogTitle>
          <DialogDescription>
            Stops the in-progress build and deploy right away. Work done so
            far is discarded. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {cancelDeploy.isError ? (
          <p className="text-sm text-destructive">
            {cancelDeploy.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            Keep running
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={cancelDeploy.isPending}
            onClick={() => {
              cancelDeploy.mutate(attemptId, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: 'Deploy cancelled.', type: 'success' })
                },
              })
            }}
          >
            {cancelDeploy.isPending ? 'Cancelling...' : 'Cancel deploy'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
