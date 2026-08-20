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
import { useDeleteOrganization } from '../queries/organizations'

// Mirrors DeleteProjectDialog exactly: the only thing worth confirming
// is that member projects survive, unmodified, simply organization-less
// again, never deleted (store.DB.DeleteOrganization's own ON DELETE SET
// NULL foreign key).
export function DeleteOrganizationDialog({
  id,
  name,
}: {
  id: string
  name: string
}) {
  const [open, setOpen] = useState(false)
  const deleteOrganization = useDeleteOrganization()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteOrganization.reset()
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
            This removes the organization label only. Any projects filed
            under it keep running exactly as they are, they just become
            organization-less again. This cannot be undone from here.
          </DialogDescription>
        </DialogHeader>
        {deleteOrganization.isError ? (
          <p className="text-sm text-destructive">
            {deleteOrganization.error.message}
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
            disabled={deleteOrganization.isPending}
            onClick={() => {
              deleteOrganization.mutate(id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Organization "${name}" deleted.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deleteOrganization.isPending ? 'Deleting...' : 'Delete organization'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
