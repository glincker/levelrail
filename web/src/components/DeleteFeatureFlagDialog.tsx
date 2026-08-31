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
import { useDeleteFeatureFlag } from '../queries/featureFlags'
import type { FeatureFlag } from '../types/featureFlags'

// One flag's delete action, mirroring DeleteScheduledTaskDialog's own
// shape: destructive and irreversible, so this is a confirm dialog, not
// a bare one-click button.
export function DeleteFeatureFlagDialog({
  appName,
  flag,
}: {
  appName: string
  flag: FeatureFlag
}) {
  const [open, setOpen] = useState(false)
  const deleteFlag = useDeleteFeatureFlag(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteFlag.reset()
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
            Delete this feature flag?
          </DialogTitle>
          <DialogDescription>
            &ldquo;{flag.key}&rdquo; stops resolving immediately: any app still
            calling the evaluate endpoint for it starts getting a 404. This
            cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteFlag.isError ? (
          <p className="text-sm text-destructive">{deleteFlag.error.message}</p>
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
            disabled={deleteFlag.isPending}
            onClick={() => {
              deleteFlag.mutate(flag.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: 'Feature flag deleted.', type: 'success' })
                },
              })
            }}
          >
            {deleteFlag.isPending ? 'Deleting...' : 'Delete flag'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
