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
import { useDeleteEnvironment } from '../queries/environments'

// Mirrors DeleteProjectDialog: the only thing worth confirming is that
// any app tagged with this environment survives, simply untagged again
// (store.DB.DeleteEnvironment's own ON DELETE SET NULL foreign key).
export function DeleteEnvironmentDialog({
  id,
  name,
  projectId,
  onDeleted,
}: {
  id: string
  name: string
  projectId: string
  onDeleted?: () => void
}) {
  const [open, setOpen] = useState(false)
  const deleteEnvironment = useDeleteEnvironment(projectId)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteEnvironment.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button variant="ghost" size="sm" aria-label={`Delete ${name}`} />}
      >
        <TrashIcon className="size-3.5" aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Any app tagged with this environment keeps running exactly as
            it is, it just becomes untagged again. This cannot be undone
            from here.
          </DialogDescription>
        </DialogHeader>
        {deleteEnvironment.isError ? (
          <p className="text-sm text-destructive">
            {deleteEnvironment.error.message}
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
            disabled={deleteEnvironment.isPending}
            onClick={() => {
              deleteEnvironment.mutate(id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Environment "${name}" deleted.`,
                    type: 'success',
                  })
                  onDeleted?.()
                },
              })
            }}
          >
            {deleteEnvironment.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
