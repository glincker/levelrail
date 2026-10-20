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
import { useDeleteNotificationChannel } from '../queries/notificationChannels'
import type { NotificationChannel } from '../types/notificationChannel'

// Mirrors DeleteBackupTargetDialog. Unlike that dialog, deleting a
// channel is never blocked: attached apps have channel_id cleared instead.
export function DeleteNotificationChannelDialog({
  channel,
}: {
  channel: NotificationChannel
}) {
  const [open, setOpen] = useState(false)
  const deleteChannel = useDeleteNotificationChannel()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteChannel.reset()
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
            Delete &ldquo;{channel.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Any app currently attached to this channel stops receiving
            notifications through it and keeps its other settings. This cannot
            be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteChannel.isError ? (
          <p className="text-sm text-destructive">
            {deleteChannel.error.message}
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
            disabled={deleteChannel.isPending}
            onClick={() => {
              deleteChannel.mutate(channel.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Channel "${channel.name}" deleted.`,
                    type: 'success',
                  })
                },
                onError: (error) => {
                  toast.add({
                    title: `Could not delete "${channel.name}".`,
                    description: error.message,
                    type: 'error',
                  })
                },
              })
            }}
          >
            {deleteChannel.isPending ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
