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
import { useDeleteRegistryCredential } from '../queries/registryCredentials'
import type { RegistryCredential } from '../types/registryCredential'

// One registry credential's delete action, the same split-out-of-the-
// table shape DeleteBackupTargetDialog already establishes. DELETE
// /api/v1/registry-credentials/{id} is destructive and irreversible,
// so this is a confirm dialog, not a bare one-click button.
export function DeleteRegistryCredentialDialog({
  credential,
}: {
  credential: RegistryCredential
}) {
  const [open, setOpen] = useState(false)
  const deleteCredential = useDeleteRegistryCredential()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteCredential.reset()
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
            Delete &ldquo;{credential.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Any service still referencing this credential in its
            registryCredential field will fail to pull a private image on
            its next deploy. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteCredential.isError ? (
          <p className="text-sm text-destructive">
            {deleteCredential.error.message}
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
            disabled={deleteCredential.isPending}
            onClick={() => {
              deleteCredential.mutate(credential.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Registry credential "${credential.name}" deleted.`,
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: `Could not delete "${credential.name}".`,
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteCredential.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
