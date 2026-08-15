import { useState } from 'react'
import { TrashIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { useDeleteDeployNotifyTarget } from '../queries/deployNotify'
import type { DeployNotifyTarget } from '../types/deployNotify'

// One target's delete action, split out of DeployNotifyTargetsPanel the
// same way DeleteAlertRuleDialog is split out of AlertRulesPanel. DELETE
// /api/v1/apps/{name}/deploy-notify-targets/{id} is destructive and
// irreversible, so this is a confirm dialog, not a bare one-click
// button, the same reasoning DeleteAlertRuleDialog's own header comment
// gives.
export function DeleteDeployNotifyTargetDialog({
  appName,
  target,
}: {
  appName: string
  target: DeployNotifyTarget
}) {
  const [open, setOpen] = useState(false)
  const deleteTarget = useDeleteDeployNotifyTarget(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteTarget.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete this deploy notify target?
          </DialogTitle>
          <DialogDescription>
            This target stops receiving deploy-outcome notifications
            immediately. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteTarget.isError ? (
          <p className="text-sm text-destructive">
            {deleteTarget.error.message}
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
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteTarget.isPending}
            onClick={() => {
              deleteTarget.mutate(target.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: 'Deploy notify target deleted.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deleteTarget.isPending ? 'Deleting...' : 'Delete target'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
