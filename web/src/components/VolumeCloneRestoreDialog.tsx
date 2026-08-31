import { useState } from 'react'
import { CopySimpleIcon } from '@phosphor-icons/react/dist/ssr'
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
import { useTriggerVolumeCloneRestore } from '../queries/volumeCloneRestore'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded app service volume backup's "restore as new volume"
// action, mirroring CloneRestoreDialog's exact shape for the database
// resource kind: the safe alternative to RestoreVolumeBackupDialog's
// in-place restore. Creates a brand-new, standalone Docker volume (not
// attached to any app or app.yaml) and restores this backup into it,
// never touching the source volume's own live contents. Rendered
// alongside RestoreVolumeBackupDialog in the same backup history row
// (AppVolumeBackupsSection.tsx), a deliberately separate action rather
// than a mode flag on the same button, the same reasoning
// CloneRestoreDialog's own doc comment gives.
//
// Unlike CloneRestoreDialog there is no project/version picker here: a
// bare Docker volume has neither. The new volume name is optional; left
// blank, the server generates one (see the started attempt's own
// new_volume_name in the success toast).
export function VolumeCloneRestoreDialog({
  appName,
  volumeName,
  backup,
}: {
  appName: string
  volumeName: string
  backup: BackupHistoryRecord
}) {
  const [open, setOpen] = useState(false)
  const [newVolumeName, setNewVolumeName] = useState('')
  const triggerCloneRestore = useTriggerVolumeCloneRestore(
    appName,
    volumeName,
  )

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setNewVolumeName('')
      triggerCloneRestore.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <CopySimpleIcon className="size-3.5" aria-hidden="true" />
        Restore as new volume
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Restore as a new volume</DialogTitle>
          <DialogDescription>
            Creates a brand-new, standalone Docker volume and restores this
            backup into it. &ldquo;{appName}/{volumeName}&rdquo;&apos;s own
            live contents are never touched. The new volume is not attached
            to any app: reference it by name to use it elsewhere.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="volume-clone-restore-new-name">
            New volume name (optional)
          </FieldLabel>
          <Input
            id="volume-clone-restore-new-name"
            placeholder="leave blank to generate a name"
            autoComplete="off"
            spellCheck={false}
            value={newVolumeName}
            onChange={(event) => {
              setNewVolumeName(event.target.value)
            }}
          />
        </Field>
        {triggerCloneRestore.isError ? (
          <p className="text-sm text-destructive">
            {triggerCloneRestore.error.message}
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
            disabled={triggerCloneRestore.isPending}
            onClick={() => {
              triggerCloneRestore.mutate(
                {
                  backup_id: backup.id,
                  new_volume_name: newVolumeName.trim() || undefined,
                },
                {
                  onSuccess: (created) => {
                    setOpen(false)
                    setNewVolumeName('')
                    toast.add({
                      title: `Restoring backup into "${created.new_volume_name}".`,
                      description:
                        'This creates the new volume, then restores this backup into it.',
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
                },
              )
            }}
          >
            {triggerCloneRestore.isPending
              ? 'Starting...'
              : 'Restore as new volume'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
