import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { Trash2Icon, TriangleAlertIcon } from 'lucide-react'
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
import { useDeleteDatabase } from '../queries/databases'

// One database's delete action, split out of the detail route the same
// way DeleteAlertRuleDialog is split out of AlertRulesPanel. DELETE
// /api/v1/databases/{name} (internal/api/databases.go's
// handleDeleteDatabase) is destructive and irreversible, there is no
// undo, so this is a confirm dialog, not a bare one-click button, same
// reasoning RevokeTokenDialog's own header comment gives. On success,
// navigates back to /databases since the detail page's own resource no
// longer exists.
export function DeleteDatabaseDialog({ name }: { name: string }) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const deleteDatabase = useDeleteDatabase()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteDatabase.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        <Trash2Icon className="size-3.5" aria-hidden="true" />
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <TriangleAlertIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            This removes the database&apos;s desired state. It does not stop or
            remove an already-running container for it. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteDatabase.isError ? (
          <p className="text-sm text-destructive">
            {deleteDatabase.error.message}
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
            disabled={deleteDatabase.isPending}
            onClick={() => {
              deleteDatabase.mutate(name, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Database "${name}" deleted.`,
                    type: 'success',
                  })
                  void navigate({ to: '/databases' })
                },
              })
            }}
          >
            {deleteDatabase.isPending ? 'Deleting...' : 'Delete database'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
