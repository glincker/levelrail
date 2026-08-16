import { useState } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from './ui/dialog'
import { Alert, AlertDescription } from './ui/alert'
import { Button } from './ui/button'
import { toast } from './ui/toast'
import { useDeleteUser } from '../queries/users'
import type { UserResource } from '../queries/users'

// DELETE /api/v1/users/{id} is destructive (revokes every session for
// that user) and irreversible from this UI, so this is a confirm
// dialog, matching RevokeTokenDialog's shape for the same reason.
export function RemoveUserDialog({ user }: { user: UserResource }) {
  const [open, setOpen] = useState(false)
  const deleteUser = useDeleteUser()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteUser.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        Remove
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon className="size-4 text-destructive" />
            Remove {user.email}?
          </DialogTitle>
          <DialogDescription>
            This ends every active session for this account immediately.
            This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteUser.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{deleteUser.error.message}</AlertDescription>
          </Alert>
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
            disabled={deleteUser.isPending}
            onClick={() => {
              deleteUser.mutate(user.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: `${user.email} removed.`, type: 'success' })
                },
              })
            }}
          >
            {deleteUser.isPending ? 'Removing...' : 'Remove'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
