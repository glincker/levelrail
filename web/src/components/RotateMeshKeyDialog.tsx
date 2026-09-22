import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { useRotateNodeMeshKey } from '../queries/mesh'

// Confirm-dialog for POST /api/v1/nodes/{id}/mesh/rotate-key, mirroring
// CordonNodeDialog's shape (open state reset on close, inline error,
// disabled-while-pending submit button): rotating a node's live
// WireGuard identity is a moderately risky operator action, the same
// class CordonNodeDialog itself calls out ("deserves a real confirm step
// rather than a bare toggle"), and here the stakes are one notch higher
// still, a brief mesh reconnect blip is a real, named possibility
// (internal/api/mesh.go's own doc comment), not just a placement change.
export function RotateMeshKeyDialog({
  nodeId,
  nodeName,
}: {
  nodeId: string
  nodeName: string
}) {
  const [open, setOpen] = useState(false)
  const rotate = useRotateNodeMeshKey()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      rotate.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
        Rotate key
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon className="size-4 text-amber-600" aria-hidden="true" />
            Rotate WireGuard key for &ldquo;{nodeName}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Generates a fresh keypair and makes it this node&apos;s live mesh
            identity immediately. Every other node picks up the new public key
            on its next reconcile pass, not instantly, so this node may see a
            brief mesh reconnect blip until the whole fleet catches up. Watch
            the rotation status below to see when it&apos;s confirmed.
          </DialogDescription>
        </DialogHeader>
        {rotate.isError ? (
          <p className="text-sm text-destructive">{rotate.error.message}</p>
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
            disabled={rotate.isPending}
            onClick={() => {
              rotate.mutate(nodeId, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Key rotated for "${nodeName}".`,
                    description:
                      'Waiting for the fleet to confirm the new key.',
                    type: 'success',
                  })
                },
              })
            }}
          >
            {rotate.isPending ? 'Rotating...' : 'Rotate key'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
