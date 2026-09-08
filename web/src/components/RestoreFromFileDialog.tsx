import { useState } from 'react'
import { UploadSimpleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { useRestoreDatabaseFromFile } from '../queries/restoreHistory'

// The uploaded-file counterpart of RestoreBackupDialog: restores a
// database from a dump file on the operator's own machine
// (POST /api/v1/databases/{name}/restore-upload,
// internal/api/database_restore_upload.go) instead of a backup this
// platform already took. The real use case is migrating an existing
// external database in for the first time, or applying a manually-taken
// dump, neither of which has a backup_history row to pick from a table,
// so this is a standalone control rather than a per-row action the way
// RestoreBackupDialog is.
//
// Same "type the database name to confirm" gate RestoreBackupDialog
// uses: this is an equally irreversible, in-place overwrite, just fed
// from a file instead of a stored backup. The confirm button also stays
// disabled until a file is actually chosen.
export function RestoreFromFileDialog({
  databaseName,
}: {
  databaseName: string
}) {
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const restoreFromFile = useRestoreDatabaseFromFile(databaseName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setConfirmText('')
      setFile(null)
      restoreFromFile.reset()
    }
  }

  const confirmed = confirmText === databaseName && file !== null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <UploadSimpleIcon className="size-3.5" aria-hidden="true" />
        Restore from a file
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Restore &ldquo;{databaseName}&rdquo; from a file?
          </DialogTitle>
          <DialogDescription>
            This overwrites &ldquo;{databaseName}&rdquo;&apos;s current data
            with the uploaded file&apos;s contents. Anything written since
            this file was created is permanently lost. This cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="restore-upload-file">Dump file</FieldLabel>
          <Input
            id="restore-upload-file"
            type="file"
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null)
            }}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="restore-upload-confirm-name">
            Type &ldquo;{databaseName}&rdquo; to confirm
          </FieldLabel>
          <Input
            id="restore-upload-confirm-name"
            autoComplete="off"
            spellCheck={false}
            value={confirmText}
            onChange={(event) => {
              setConfirmText(event.target.value)
            }}
          />
        </Field>
        {restoreFromFile.isError ? (
          <p className="text-sm text-destructive">
            {restoreFromFile.error.message}
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
            disabled={!confirmed || restoreFromFile.isPending}
            onClick={() => {
              if (!file) return
              restoreFromFile.mutate(file, {
                onSuccess: () => {
                  setOpen(false)
                  setConfirmText('')
                  setFile(null)
                  toast.add({
                    title: 'Restore finished.',
                    description: `"${databaseName}" was restored from the uploaded file.`,
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: 'Restore failed.',
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {restoreFromFile.isPending ? 'Restoring...' : 'Restore database'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
