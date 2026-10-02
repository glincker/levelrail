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
import { useDeleteNetworkShare } from '../queries/networkShares'
import type { NetworkShare } from '../types/networkShare'

// One network share's delete action, the same split-out-of-the-table
// shape DeleteRegistryCredentialDialog already establishes. DELETE
// /api/v1/network-shares/{id} is destructive and irreversible, so this
// is a confirm dialog, not a bare one-click button.
export function DeleteNetworkShareDialog({ share }: { share: NetworkShare }) {
  const [open, setOpen] = useState(false)
  const deleteShare = useDeleteNetworkShare()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteShare.reset()
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
            Delete &ldquo;{share.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Any app volume still mounting this share will fail to start on its
            next deploy. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteShare.isError ? (
          <p className="text-sm text-destructive">
            {deleteShare.error.message}
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
            disabled={deleteShare.isPending}
            onClick={() => {
              deleteShare.mutate(share.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Network share "${share.name}" deleted.`,
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: `Could not delete "${share.name}".`,
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteShare.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
