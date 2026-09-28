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
import { useDeleteModel } from '../queries/models'

// DELETE /api/v1/models/{name} removes the engine container; the
// downloaded weights volume is kept so a redeploy does not download again.
export function DeleteModelDialog({
  name,
  disabled,
}: {
  name: string
  disabled?: boolean
}) {
  const [open, setOpen] = useState(false)
  const deleteModel = useDeleteModel()

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) deleteModel.reset()
      }}
    >
      <DialogTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={`Delete ${name}`}
            title="Delete"
            disabled={disabled}
          />
        }
      >
        <TrashIcon aria-hidden="true" />
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Delete &ldquo;{name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            The engine container is removed and the API key stops working. The
            downloaded weights are kept, so deploying the same model again does
            not download it twice.
          </DialogDescription>
        </DialogHeader>
        {deleteModel.isError ? (
          <p className="text-sm text-destructive">
            {deleteModel.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteModel.isPending}
            onClick={() => {
              deleteModel.mutate(name, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Model "${name}" deleted.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {deleteModel.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
