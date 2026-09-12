import { useState } from 'react'
import type { UseMutationResult } from '@tanstack/react-query'
import {
  ClockCounterClockwiseIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useTriggerRestore } from '../queries/restoreHistory'
import type { ApiError } from '../lib/apiError'
import type { BackupHistoryRecord } from '../types/backupHistory'
import type { RestoreHistoryRecord } from '../types/restoreHistory'

// One succeeded backup's restore action, triggered from a row in
// BackupsSection's history table. POST /api/v1/databases/{name}/restore
// (internal/api/restore.go's handleTriggerRestore) is the single most
// destructive endpoint in this API: it overwrites the database's live
// data in place, with no undo beyond restoring again from a different
// backup. DeleteAppDialog/DeleteDatabaseDialog/DeleteBackupTargetDialog
// all gate their own, lower-stakes destructive actions behind a confirm
// dialog with a Cancel/Delete button pair; this dialog goes one step
// further, the same way handleTriggerRestore's own AbilityRoot gate goes
// one step further than those routes' AbilityWrite/AbilityWriteSensitive:
// the confirm button stays disabled until the operator types the
// database's own name into a field, exactly matching it, character for
// character. A misclick can't fire this request; only a deliberate,
// readable action can.
export function RestoreBackupDialog({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  const triggerRestore = useTriggerRestore(databaseName)
  return (
    <ConfirmRestoreDialog
      target={databaseName}
      confirmInputId="restore-confirm-name"
      subjectLabel="data"
      confirmButtonLabel="Restore database"
      backupId={backup.id}
      triggerRestore={triggerRestore}
    />
  )
}

// Shared between RestoreBackupDialog and RestoreVolumeBackupDialog: same
// type-the-target-name-to-confirm dialog for both resource kinds, only
// the target string, field labels, and the mutation backing "Restore"
// differ between callers.
export function ConfirmRestoreDialog({
  target,
  confirmInputId,
  subjectLabel,
  confirmButtonLabel,
  backupId,
  triggerRestore,
}: {
  target: string
  confirmInputId: string
  subjectLabel: string
  confirmButtonLabel: string
  backupId: string
  triggerRestore: UseMutationResult<RestoreHistoryRecord, ApiError, string>
}) {
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setConfirmText('')
      triggerRestore.reset()
    }
  }

  const confirmed = confirmText === target

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <ClockCounterClockwiseIcon className="size-3.5" aria-hidden="true" />
        Restore
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Restore &ldquo;{target}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This overwrites &ldquo;{target}&rdquo;&apos;s current{' '}
            {subjectLabel} with this backup&apos;s contents. Anything written
            since this backup was taken is permanently lost. This cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor={confirmInputId}>
            Type &ldquo;{target}&rdquo; to confirm
          </FieldLabel>
          <Input
            id={confirmInputId}
            autoComplete="off"
            spellCheck={false}
            value={confirmText}
            onChange={(event) => {
              setConfirmText(event.target.value)
            }}
          />
        </Field>
        {triggerRestore.isError ? (
          <p className="text-sm text-destructive">
            {triggerRestore.error.message}
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
            disabled={!confirmed || triggerRestore.isPending}
            onClick={() => {
              triggerRestore.mutate(backupId, {
                onSuccess: () => {
                  setOpen(false)
                  setConfirmText('')
                  toast.add({
                    title: 'Restore started.',
                    description:
                      'This list updates automatically once it finishes.',
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: 'Could not start restore.',
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {triggerRestore.isPending ? 'Starting...' : confirmButtonLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
