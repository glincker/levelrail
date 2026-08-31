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
import { useTriggerCloneRestore } from '../queries/cloneRestore'
import { useProjectListOptional } from '../queries/projects'
import {
  NO_PROJECT_VALUE,
  ProjectSelectField,
} from './PlacementFields'
import type { BackupHistoryRecord } from '../types/backupHistory'

// One succeeded backup's "restore as new database" action, the safe
// alternative to RestoreBackupDialog's in-place restore: creates a
// brand-new database and restores this backup into it, never touching
// databaseName's own live data. Rendered alongside RestoreBackupDialog in
// the same backup history row (BackupsSection.tsx), a deliberately
// separate action rather than a mode flag on the same button/endpoint:
// this is safe enough to not need RestoreBackupDialog's "type the name to
// confirm" gate, and conflating the two behind one control would make the
// safe action look as dangerous as the destructive one, or worse, make
// the destructive one look as safe as this one.
export function CloneRestoreDialog({
  databaseName,
  backup,
}: {
  databaseName: string
  backup: BackupHistoryRecord
}) {
  const [open, setOpen] = useState(false)
  const [newName, setNewName] = useState('')
  const [project, setProject] = useState(NO_PROJECT_VALUE)
  const triggerCloneRestore = useTriggerCloneRestore(databaseName)
  const projectList = useProjectListOptional()
  const projects = projectList.data ?? []

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setNewName('')
      setProject(NO_PROJECT_VALUE)
      triggerCloneRestore.reset()
    }
  }

  const trimmedName = newName.trim()
  const canSubmit = trimmedName.length > 0 && trimmedName !== databaseName

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <CopySimpleIcon className="size-3.5" aria-hidden="true" />
        Restore as new
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Restore as a new database</DialogTitle>
          <DialogDescription>
            Creates a brand-new database and restores this backup into it.
            &ldquo;{databaseName}&rdquo;&apos;s own live data is never
            touched.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="clone-restore-new-name">
            New database name
          </FieldLabel>
          <Input
            id="clone-restore-new-name"
            placeholder={`e.g. ${databaseName}-staging`}
            autoComplete="off"
            spellCheck={false}
            value={newName}
            onChange={(event) => {
              setNewName(event.target.value)
            }}
          />
        </Field>
        <ProjectSelectField
          idPrefix="clone-restore"
          projects={projects}
          value={project}
          onValueChange={setProject}
        />
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
            disabled={!canSubmit || triggerCloneRestore.isPending}
            onClick={() => {
              triggerCloneRestore.mutate(
                {
                  backup_id: backup.id,
                  new_name: trimmedName,
                  project_id:
                    project === NO_PROJECT_VALUE ? undefined : project,
                },
                {
                  onSuccess: (created) => {
                    setOpen(false)
                    setNewName('')
                    setProject(NO_PROJECT_VALUE)
                    toast.add({
                      title: `Restoring backup into "${created.new_database_name}".`,
                      description:
                        'This creates and starts the new database, then restores this backup into it once it is up.',
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
              : 'Restore as new database'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
