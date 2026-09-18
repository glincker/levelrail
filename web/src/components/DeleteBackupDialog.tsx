import { useState } from 'react'
import type { UseMutationResult } from '@tanstack/react-query'
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
import { useDeleteBackup } from '../queries/backupHistory'
import { useDeleteVolumeBackup } from '../queries/volumeBackupHistory'
import type { ApiError } from '../lib/apiError'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One archived backup's delete action, split out the same way
// DeleteBackupTargetDialog is split out of BackupTargetTable. DELETE
// .../backups/{historyId} (handleDeleteBackup/handleDeleteVolumeBackup) is
// destructive and irreversible: once gone, the stored archive cannot be
// recovered. Uses DeleteBackupTargetDialog's own confirm-dialog shape
// (Cancel/Delete, no typed name) rather than RestoreBackupDialog's
// type-to-confirm one: deleting one archive is the same AbilityWriteSensitive
// risk tier server-side as disconnecting a backup target, not the
// AbilityRoot tier a live restore carries, so it gets that tier's own
// confirmation weight, not a heavier one.
export function ConfirmDeleteBackupDialog({
  backupId,
  deleteBackup,
}: {
  backupId: string
  deleteBackup: UseMutationResult<void, ApiError, string>
}) {
  const [open, setOpen] = useState(false)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteBackup.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete this backup?
          </DialogTitle>
          <DialogDescription>
            This permanently deletes this one archived backup, both the
            stored file and its history entry. The rest of this backup
            history and any recurring schedule are untouched. This cannot
            be undone.
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
              deleteBackup.mutate(backupId, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: 'Backup deleted.',
                    type: 'success',
                  })
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

export function DeleteBackupDialog({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  const deleteBackup = useDeleteBackup(databaseName)
  return (
    <ConfirmDeleteBackupDialog backupId={backup.id} deleteBackup={deleteBackup} />
  )
}

export function DeleteVolumeBackupDialog({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  const deleteBackup = useDeleteVolumeBackup(appName, volumeName)
  return (
    <ConfirmDeleteBackupDialog backupId={backup.id} deleteBackup={deleteBackup} />
  )
}
