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
import { useCloneDatabaseNow } from '../queries/cloneNow'

// The one-click counterpart to CloneRestoreDialog: that dialog restores
// an already-succeeded backup into a new database, so it only makes
// sense per backup-history row and needs one to exist first. This one
// takes a fresh backup as part of the same action, so it's a single,
// standalone control (BackupsSection.tsx), not nested in the backup
// history table, and works even for a database with no backup history
// yet.
export function CloneNowDialog({ databaseName }: { databaseName: string }) {
  const [open, setOpen] = useState(false)
  const [newName, setNewName] = useState('')
  const cloneNow = useCloneDatabaseNow(databaseName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setNewName('')
      cloneNow.reset()
    }
  }

  const trimmedName = newName.trim()
  const canSubmit = trimmedName.length > 0 && trimmedName !== databaseName

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <CopySimpleIcon className="size-3.5" aria-hidden="true" />
        Clone now
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Clone this database</DialogTitle>
          <DialogDescription>
            Takes a fresh backup of &ldquo;{databaseName}&rdquo; and
            restores it into a brand-new database, in one step. No
            existing backup is required, and &ldquo;{databaseName}
            &rdquo;&apos;s own live data is never touched.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="clone-now-new-name">
            New database name
          </FieldLabel>
          <Input
            id="clone-now-new-name"
            placeholder={`e.g. ${databaseName}-staging`}
            autoComplete="off"
            spellCheck={false}
            value={newName}
            onChange={(event) => {
              setNewName(event.target.value)
            }}
          />
        </Field>
        {cloneNow.isError ? (
          <p className="text-sm text-destructive">{cloneNow.error.message}</p>
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
            disabled={!canSubmit || cloneNow.isPending}
            onClick={() => {
              cloneNow.mutate(
                { new_name: trimmedName },
                {
                  onSuccess: (started) => {
                    setOpen(false)
                    setNewName('')
                    toast.add({
                      title: `Cloning into "${started.new_database_name}".`,
                      description:
                        'This takes a backup now, then restores it into the new database once it is up.',
                      type: 'success',
                    })
                  },
                  onError: (error) => {
                    toast.add({
                      title: 'Could not start clone.',
                      description: error.message,
                      type: 'error',
                    })
                  },
                },
              )
            }}
          >
            {cloneNow.isPending ? 'Starting...' : 'Clone now'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
