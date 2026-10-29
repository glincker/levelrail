import { TrashGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRemoveOrphanedContainer } from '../queries/containers'

export function RemoveContainerDialog({
  containerName,
  open,
  onOpenChange,
}: {
  containerName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const removeContainer = useRemoveOrphanedContainer()

  function handleOpenChange(next: boolean) {
    onOpenChange(next)
    if (!next) removeContainer.reset()
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <TriangleAlertGlassIcon
              className="size-4.5 text-tone-warning"
              aria-hidden="true"
            />
            Remove &ldquo;{containerName}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Stops and deletes this container. It is not managed by Levelrail, so
            nothing recreates it. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {removeContainer.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>{removeContainer.error.message}</AlertDescription>
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
            disabled={removeContainer.isPending}
            onClick={() => {
              removeContainer.mutate(containerName, {
                onSuccess: () => {
                  onOpenChange(false)
                  toast.add({
                    title: `Removed container "${containerName}".`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            <TrashGlassIcon className="size-4" aria-hidden="true" />
            {removeContainer.isPending ? 'Removing...' : 'Remove container'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
