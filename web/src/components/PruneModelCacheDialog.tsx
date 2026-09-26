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
import { formatBytes } from '../lib/modelPreflight'
import { usePruneModelCache } from '../queries/modelPreflight'

// Two steps: opening the dialog runs a dry run and lists exactly what would
// go, and the confirm button removes only those volumes. The server rechecks
// every one and keeps any that a model or container has started using since.
export function PruneModelCacheDialog({
  reclaimableBytes,
  unusedDays,
}: {
  reclaimableBytes: number
  unusedDays: number
}) {
  const [open, setOpen] = useState(false)
  const preview = usePruneModelCache()
  const prune = usePruneModelCache()
  const candidates = preview.data?.candidates ?? []

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      preview.mutate({ dryRun: true })
    } else {
      preview.reset()
      prune.reset()
    }
  }

  function confirm() {
    prune.mutate(
      { dryRun: false, volumes: candidates.map((c) => c.volume) },
      {
        onSuccess: (result) => {
          toast.add({
            title: `Removed ${result.removed.length} cached model volume${result.removed.length === 1 ? '' : 's'}.`,
            description: `Reclaimed ${formatBytes(result.reclaimed_bytes)}.`,
            type: 'success',
          })
          handleOpenChange(false)
        },
      },
    )
  }

  const failure = preview.isError
    ? preview.error.message
    : prune.isError
      ? prune.error.message
      : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button
            size="sm"
            variant="outline"
            disabled={reclaimableBytes <= 0}
          />
        }
      >
        <BroomIcon aria-hidden="true" />
        Prune unused
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon className="size-4" aria-hidden="true" />
            Prune unused model weights?
          </DialogTitle>
          <DialogDescription>
            Removes cached weights that no model uses and that were unused for
            over {unusedDays} days. Weights of any deployed model, or mounted by
            a running container, are never removed. Removed weights download
            again if you deploy that model later.
          </DialogDescription>
        </DialogHeader>
        {preview.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            Checking what would be removed...
          </p>
        ) : null}
        {preview.isSuccess && candidates.length === 0 ? (
          <p role="status" className="text-sm text-muted-foreground">
            Nothing to prune right now.
          </p>
        ) : null}
        {candidates.length > 0 ? (
          <ul
            aria-label="Volumes that would be removed"
            className="max-h-48 space-y-1 overflow-auto rounded-md border border-border p-2 text-xs"
          >
            {candidates.map((c) => (
              <li key={c.volume} className="flex justify-between gap-3">
                <span className="truncate font-mono">{c.volume}</span>
                <span className="text-muted-foreground">
                  {c.size_bytes === null
                    ? 'size unknown'
                    : formatBytes(c.size_bytes)}
                </span>
              </li>
            ))}
          </ul>
        ) : null}
        {failure ? (
          <p role="alert" className="text-sm text-destructive">
            {failure}
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
            disabled={candidates.length === 0 || prune.isPending}
            onClick={confirm}
          >
            {prune.isPending
              ? 'Removing...'
              : `Remove ${candidates.length} volume${candidates.length === 1 ? '' : 's'}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
