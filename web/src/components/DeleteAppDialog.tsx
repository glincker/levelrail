import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
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
import { useDeleteApp } from '../queries/apps'

// One app's delete action, split out of the detail route the same way
// DeleteDatabaseDialog is split out of routes/databases/$name.tsx. DELETE
// /api/v1/apps/{name} (internal/api/apps.go's handleDeleteApp) is
// destructive and irreversible, there is no undo, so this is a confirm
// dialog, not a bare one-click button, same reasoning
// DeleteDatabaseDialog's own header comment gives. On success, navigates
// back to /apps since the detail page's own resource no longer exists.
export function DeleteAppDialog({ name }: { name: string }) {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const deleteApp = useDeleteApp()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteApp.reset()
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
            This removes the app&apos;s desired state. It does not stop or
            remove an already-running container for it. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteApp.isError ? (
          <p className="text-sm text-destructive">{deleteApp.error.message}</p>
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
            disabled={deleteApp.isPending}
            onClick={() => {
              deleteApp.mutate(name, {
                onSuccess: () => {
                  setOpen(false)
                  void navigate({ to: '/apps' })
                },
              })
            }}
          >
            {deleteApp.isPending ? 'Deleting...' : 'Delete app'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
