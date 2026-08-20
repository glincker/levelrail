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
import { useDeleteScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'

// One task's delete action, mirroring DeleteAlertRuleDialog's own shape:
// destructive and irreversible (no undo, no history kept once gone), so
// this is a confirm dialog, not a bare one-click button.
export function DeleteScheduledTaskDialog({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const [open, setOpen] = useState(false)
  const deleteTask = useDeleteScheduledTask(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      deleteTask.reset()
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
            Delete this scheduled task?
          </DialogTitle>
          <DialogDescription>
            &ldquo;{task.schedule}&rdquo; stops running immediately. This
            cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {deleteTask.isError ? (
          <p className="text-sm text-destructive">{deleteTask.error.message}</p>
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
            disabled={deleteTask.isPending}
            onClick={() => {
              deleteTask.mutate(task.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({ title: 'Scheduled task deleted.', type: 'success' })
                },
              })
            }}
          >
            {deleteTask.isPending ? 'Deleting...' : 'Delete task'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
