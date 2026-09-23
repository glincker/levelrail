import { useState } from 'react'
import { HardDrivesIcon, TrashIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { formatBytes } from '../lib/format'
import {
  useCleanupOrphanedVolumes,
  useOrphanedVolumes,
} from '../queries/orphanedVolumes'

// OrphanedVolumesCard is the System Prune page's companion for a gap
// POST /system/prune deliberately never closes: a named Docker volume
// (an app's storage attachment, a database's data volume) whose owning
// app/database/storage attachment was deleted is never touched by that
// endpoint (internal/docker/prune.go's PruneAnonymousVolumes only ever
// removes Docker's own anonymous volumes, on purpose, so a real
// database volume is never at risk from an operator clicking "clean up
// now"). This card is the detection-first surface for that other,
// equally real leak: list what's actually orphaned, let the operator
// pick which ones, confirm, then delete exactly those.
export function OrphanedVolumesCard() {
  const { data: volumes, isLoading } = useOrphanedVolumes()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [confirmOpen, setConfirmOpen] = useState(false)
  const cleanup = useCleanupOrphanedVolumes()

  if (isLoading) {
    return null
  }
  if (!volumes || volumes.length === 0) {
    return null
  }

  function toggle(name: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) {
        next.delete(name)
      } else {
        next.add(name)
      }
      return next
    })
  }

  const selectedNames = [...selected].filter((name) =>
    volumes.some((v) => v.name === name),
  )

  function handleConfirmOpenChange(next: boolean) {
    setConfirmOpen(next)
    if (!next) {
      cleanup.reset()
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <HardDrivesIcon className="size-4" />
            </div>
            <div>
              <CardTitle>Orphaned volumes</CardTitle>
              <CardDescription>
                Named volumes whose app or database was deleted. Never
                touched by &ldquo;Clean up now&rdquo; above; review and
                remove them here.
              </CardDescription>
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-1 divide-y divide-border">
        {volumes.map((v) => (
          <Label
            key={v.name}
            className="flex cursor-pointer items-center justify-between gap-3 py-2 font-normal"
          >
            <span className="flex items-center gap-2 min-w-0">
              <Checkbox
                checked={selected.has(v.name)}
                onCheckedChange={() => {
                  toggle(v.name)
                }}
              />
              <span className="truncate font-mono text-sm text-foreground">
                {v.name}
              </span>
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {formatBytes(v.size_bytes)}
            </span>
          </Label>
        ))}
      </CardContent>
      <CardContent className="flex items-center justify-between gap-3 pt-0">
        <p className="text-xs text-muted-foreground">
          {selectedNames.length > 0
            ? `${selectedNames.length} selected`
            : `${volumes.length} orphaned volume(s) found`}
        </p>
        <Dialog open={confirmOpen} onOpenChange={handleConfirmOpenChange}>
          <DialogTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={selectedNames.length === 0}
              />
            }
          >
            <TrashIcon className="size-3.5" aria-hidden="true" />
            Clean up selected
          </DialogTrigger>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <WarningIcon className="size-4" aria-hidden="true" />
                Delete {selectedNames.length} volume
                {selectedNames.length === 1 ? '' : 's'}?
              </DialogTitle>
              <DialogDescription render={<div className="space-y-2 text-left" />}>
                <p>This permanently deletes:</p>
                <ul className="max-h-40 list-disc space-y-1 overflow-y-auto pl-5 font-mono text-xs">
                  {selectedNames.map((name) => (
                    <li key={name}>{name}</li>
                  ))}
                </ul>
                <p>
                  Each one is re-checked right before deletion; if an app or
                  database started referencing it again in the meantime, it
                  is skipped instead. This cannot be undone.
                </p>
              </DialogDescription>
            </DialogHeader>
            {cleanup.isError ? (
              <p className="text-sm text-destructive">
                {cleanup.error.message}
              </p>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleConfirmOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                disabled={cleanup.isPending}
                onClick={() => {
                  cleanup.mutate(selectedNames, {
                    onSuccess: (result) => {
                      setConfirmOpen(false)
                      setSelected(new Set())
                      if (result.errors && result.errors.length > 0) {
                        toast.add({
                          title: `Clean up finished with ${result.errors.length} issue(s). Removed ${result.removed.length}.`,
                          type: 'error',
                        })
                        return
                      }
                      const skippedNote =
                        result.skipped && result.skipped.length > 0
                          ? ` (${result.skipped.length} no longer orphaned, skipped)`
                          : ''
                      toast.add({
                        title: `Removed ${result.removed.length} volume(s), reclaimed ${formatBytes(result.reclaimed_bytes)}${skippedNote}.`,
                        type: 'success',
                      })
                    },
                  })
                }}
              >
                {cleanup.isPending ? 'Deleting...' : 'Delete'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
