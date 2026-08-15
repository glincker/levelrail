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
import { useDeleteBackupTarget } from '../queries/backupTargets'
import type { BackupTarget } from '../types/backupTarget'

// One backup target's delete action, split out of BackupTargetTable the
// same way RevokeTokenDialog is split out of TokenTable. DELETE
// /api/v1/backup-targets/{id} (handleDeleteBackupTarget) is destructive
// and irreversible, there is no undo, so this is a confirm dialog, not
// a bare one-click button, same reasoning every other delete dialog in
// this codebase gives.
//
// Unlike DeleteAppDialog/DeleteDatabaseDialog, a failed delete here has
// a real, reachable non-500 outcome worth naming explicitly: a 409 if
// backup_history still references this target (handleDeleteBackupTarget's
// own doc comment). Nothing creates history yet, so this can't actually
// happen today, but the dialog still treats it as a normal, surfaced
// failure rather than assuming delete always succeeds, per the task's
// explicit requirement.
export function DeleteBackupTargetDialog({ target }: { target: BackupTarget }) {
  const [open, setOpen] = useState(false)
  const deleteBackupTarget = useDeleteBackupTarget()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteBackupTarget.reset()
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
            Delete &ldquo;{target.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This removes the connection to this bucket. Nothing already backed
            up there is deleted. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteBackupTarget.isError ? (
          <p className="text-sm text-destructive">
            {deleteBackupTarget.error.message}
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
            disabled={deleteBackupTarget.isPending}
            onClick={() => {
              deleteBackupTarget.mutate(target.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Backup target "${target.name}" deleted.`,
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: `Could not delete "${target.name}".`,
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteBackupTarget.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
