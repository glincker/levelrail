import { useState } from 'react'
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
import { useTriggerVolumeRestore } from '../queries/volumeRestoreHistory'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded app service volume backup's restore action, mirroring
// RestoreBackupDialog's exact shape for the database resource kind:
// the same one-step-further-than-an-ordinary-delete-confirm dialog,
// requiring the operator to type the exact target string before the
// confirm button enables at all. A volume has no single global name the
// way a database does, so the string to type is "<app>/<volume>", the
// same composite identifier the CLI's own --confirm flag requires
// (cmd/levelrail-cli/app_volume_backups_restore.go).
export function RestoreVolumeBackupDialog({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  const target = `${appName}/${volumeName}`
  const [open, setOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const triggerRestore = useTriggerVolumeRestore(appName, volumeName)

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
            This overwrites &ldquo;{target}&rdquo;&apos;s current volume
            contents with this backup&apos;s contents. Anything written since
            this backup was taken is permanently lost. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="volume-restore-confirm-name">
            Type &ldquo;{target}&rdquo; to confirm
          </FieldLabel>
          <Input
            id="volume-restore-confirm-name"
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
              triggerRestore.mutate(backup.id, {
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
            {triggerRestore.isPending ? 'Starting...' : 'Restore volume'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
