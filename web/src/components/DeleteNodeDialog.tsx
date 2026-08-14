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
import { useDeleteNode } from '../queries/nodes'

// One node's delete action, mirroring DeleteDatabaseDialog's shape:
// destructive, irreversible, so a confirm dialog rather than a bare
// one-click button. Unlike DeleteDatabaseDialog there is no detail route
// to navigate away from, this node's row simply disappears from the
// list on success (useDeleteNode already invalidates nodeKeys.list()).
//
// The description below states plainly what deleteNode's own doc
// comment in queries/nodes.ts says: DELETE /api/v1/nodes/{id}
// (internal/api/nodes.go's handleDeleteNode) removes the registry row
// only. It does not drain the node or disconnect a real agent session
// (TASKS.md 3.7, not built), so an operator must not read "delete" here
// as "safely decommission this machine."
export function DeleteNodeDialog({ id, name }: { id: string; name: string }) {
  const [open, setOpen] = useState(false)
  const deleteNode = useDeleteNode()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteNode.reset()
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
            This removes the node from this control plane&apos;s registry. It
            does not drain the node or stop its agent connection: if the agent
            process is still running on that machine, it stays connected and its
            workloads keep running. This cannot be undone from here.
          </DialogDescription>
        </DialogHeader>
        {deleteNode.isError ? (
          <p className="text-sm text-destructive">{deleteNode.error.message}</p>
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
            disabled={deleteNode.isPending}
            onClick={() => {
              deleteNode.mutate(id, {
                onSuccess: () => {
                  setOpen(false)
                },
              })
            }}
          >
            {deleteNode.isPending ? 'Deleting...' : 'Delete node'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
