import { useState } from 'react'
import { BroomIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { formatBytes } from '../lib/format'
import { useTriggerSystemPrune } from '../queries/systemPrune'

// POST /api/v1/system/prune's frontend trigger, the General settings
// page's "Clean up now" action. Unlike RestartAppButton (a single app,
// non-destructive: the reconciler proves the new container healthy
// before removing the old one), this deletes real Docker resources
// fleet-wide with no undo, the same "destructive, needs a confirm
// dialog, not a bare button" reasoning DeleteAppDialog's own header
// comment gives for app deletion. The dialog body states plainly what
// this does and doesn't touch, mirroring internal/docker/prune.go's own
// safety design so an operator isn't guessing: only genuinely unused
// resources go, nothing this control plane still considers desired.
export function CleanUpDockerDialog() {
  const [open, setOpen] = useState(false)
  const triggerPrune = useTriggerSystemPrune()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      triggerPrune.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <BroomIcon className="size-3.5" aria-hidden="true" />
        Clean up now
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon className="size-4" aria-hidden="true" />
            Clean up Docker storage?
          </DialogTitle>
          <DialogDescription render={<div className="space-y-2 text-left" />}>
            <p>This removes:</p>
            <ul className="list-disc space-y-1 pl-5">
              <li>Stopped containers no longer needed by any app</li>
              <li>Dangling images (untagged, unused build leftovers)</li>
              <li>Anonymous volumes not attached to any container</li>
              <li>Unused BuildKit build cache</li>
            </ul>
            <p>
              It never removes a currently running container, a tagged image
              (including previous versions kept for rollback), or a named
              database volume. This cannot be undone.
            </p>
          </DialogDescription>
        </DialogHeader>
        {triggerPrune.isError ? (
          <p className="text-sm text-destructive">
            {triggerPrune.error.message}
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
            disabled={triggerPrune.isPending}
            onClick={() => {
              triggerPrune.mutate(undefined, {
                onSuccess: (result) => {
                  setOpen(false)
                  const reclaimed =
                    result.containers_reclaimed_bytes +
                    result.images_reclaimed_bytes +
                    result.volumes_reclaimed_bytes +
                    result.build_cache_reclaimed_bytes
                  if (result.errors && result.errors.length > 0) {
                    toast.add({
                      title: `Clean up finished with ${result.errors.length} issue(s). Reclaimed ${formatBytes(reclaimed)}.`,
                      type: 'error',
                    })
                    return
                  }
                  toast.add({
                    title:
                      reclaimed > 0
                        ? `Clean up complete. Reclaimed ${formatBytes(reclaimed)}.`
                        : 'Clean up complete. Nothing to reclaim.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            {triggerPrune.isPending ? 'Cleaning up...' : 'Clean up now'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
