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
import { useDeleteBackupHistory } from '../queries/backupHistory'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One backup history row's delete action: DELETE
// /api/v1/databases/{name}/backups/{historyId} (handleDeleteBackupHistory),
// separate from the existing retention policy (BackupScheduleForm), for a
// bad, accidental, or no-longer-needed attempt an operator wants gone now.
// Irreversible, so this still confirms, the same reasoning
// DeleteBackupTargetDialog gives, but scoped down to a single table row:
// an icon-only trigger rather than a labeled button, since this sits
// inline in BackupHistoryTable next to Download/Restore/Clone, not as a
// standalone destructive action on its own card.
export function DeleteBackupHistoryDialog({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  const [open, setOpen] = useState(false)
  const deleteBackup = useDeleteBackupHistory(databaseName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteBackup.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Delete this backup"
          />
        }
      >
        <TrashIcon className="size-3.5" aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete this backup?
          </DialogTitle>
          <DialogDescription>
            This removes the backup from history. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteBackup.isError ? (
          <p className="text-sm text-destructive">
            {deleteBackup.error.message}
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
            disabled={deleteBackup.isPending}
            onClick={() => {
              deleteBackup.mutate(backup.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: 'Backup deleted.', type: 'success' })
                },
                onError: (error) => {
                  toast.add({
                    title: 'Could not delete backup.',
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteBackup.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
