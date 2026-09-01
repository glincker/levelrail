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
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useDeletePolicy } from '../queries/iamPolicies'
import type { PolicyResource } from '../queries/iamPolicies'

// DELETE /api/v1/iam/policies/{id} is idempotent and irreversible (no
// undo endpoint), same reasoning RevokeTokenDialog gives for tokens, so
// this is a confirm dialog rather than a bare one-click button.
export function DeletePolicyDialog({ policy }: { policy: PolicyResource }) {
  const [open, setOpen] = useState(false)
  const deletePolicy = useDeletePolicy()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deletePolicy.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        Delete
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <WarningIcon className="size-4 text-destructive" />
            Delete &ldquo;{policy.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Every user and token this policy is attached to loses its Allow/
            Deny rules immediately. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deletePolicy.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{deletePolicy.error.message}</AlertDescription>
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
            disabled={deletePolicy.isPending}
            onClick={() => {
              deletePolicy.mutate(policy.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Policy "${policy.name}" deleted.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deletePolicy.isPending ? 'Deleting...' : 'Delete policy'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
