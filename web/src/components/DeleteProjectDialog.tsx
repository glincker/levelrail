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
import { useDeleteProject } from '../queries/projects'

// One project's delete action, mirroring DeleteNodeDialog's confirm-
// dialog shape. The description states plainly what
// store.DB.DeleteProject's own doc comment (and handleDeleteProject's)
// says: this only removes the project label. Every app and database
// that belonged to it keeps running, unmodified, simply project-less
// again, never deleted or stopped. That's the one thing worth a confirm
// dialog at all here, since the delete itself is otherwise harmless.
export function DeleteProjectDialog({
  id,
  name,
  onDeleted,
}: {
  id: string
  name: string
  /** Called after a successful delete, so a caller on the project's own
   *  detail page (which no longer has anything to show once its project
   *  is gone) can navigate away; the list page doesn't need this, its
   *  row just disappears via useDeleteProject's own list invalidation. */
  onDeleted?: () => void
}) {
  const [open, setOpen] = useState(false)
  const deleteProject = useDeleteProject()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteProject.reset()
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
            Delete &ldquo;{name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This removes the project label only. Any apps or databases filed
            under it keep running exactly as they are, they just become
            project-less again. This cannot be undone from here.
          </DialogDescription>
        </DialogHeader>
        {deleteProject.isError ? (
          <p className="text-sm text-destructive">
            {deleteProject.error.message}
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
            disabled={deleteProject.isPending}
            onClick={() => {
              deleteProject.mutate(id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Project "${name}" deleted.`,
                    type: 'success',
                  })
                  onDeleted?.()
                },
              })
            }}
          >
            {deleteProject.isPending ? 'Deleting...' : 'Delete project'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
