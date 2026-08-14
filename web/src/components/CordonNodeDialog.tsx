import { useState } from 'react'
import {
  LockIcon,
  LockOpenIcon,
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
import { useCordonNode, useUncordonNode } from '../queries/nodes'
import type { NodeResource } from '../types/nodeDetail'

// Cordon/uncordon toggle for the node detail route
// (routes/nodes/$id.tsx). Neither direction is as irreversible as
// delete, but cordoning blocks all new placement onto a real machine
// and is exactly the kind of operator action that deserves a real
// confirm step rather than a bare toggle switch, mirroring
// DeleteNodeDialog's confirm-dialog shape (open state reset on close,
// inline error, disabled-while-pending submit button) rather than
// AppRow's own DeployStrategySelect-style silent-write control.
//
// POST /api/v1/nodes/{id}/cordon and /uncordon
// (internal/api/nodes.go's handleCordonNode/handleUncordonNode) are
// both idempotent, so there is no failure mode from double-clicking or
// from the node already being in the target state by the time this
// request lands.
export function CordonNodeDialog({ node }: { node: NodeResource }) {
  const [open, setOpen] = useState(false)
  const cordon = useCordonNode()
  const uncordon = useUncordonNode()
  const mutation = node.schedulable ? cordon : uncordon

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      cordon.reset()
      uncordon.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        {node.schedulable ? (
          <>
            <LockIcon className="size-3.5" aria-hidden="true" />
            Cordon
          </>
        ) : (
          <>
            <LockOpenIcon className="size-3.5" aria-hidden="true" />
            Uncordon
          </>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon className="size-4 text-amber-600" aria-hidden="true" />
            {node.schedulable ? 'Cordon' : 'Uncordon'} &ldquo;{node.name}
            &rdquo;?
          </DialogTitle>
          <DialogDescription>
            {node.schedulable
              ? 'This node stops accepting new app and database placements immediately. Everything already running on it keeps running unaffected. Use drain separately to move existing workloads off.'
              : 'This node starts accepting new app and database placements again. It does not move anything back onto this node automatically.'}
          </DialogDescription>
        </DialogHeader>
        {mutation.isError ? (
          <p className="text-sm text-destructive">{mutation.error.message}</p>
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
            disabled={mutation.isPending}
            onClick={() => {
              mutation.mutate(node.id, {
                onSuccess: () => {
                  setOpen(false)
                },
              })
            }}
          >
            {mutation.isPending
              ? 'Saving...'
              : node.schedulable
                ? 'Cordon node'
                : 'Uncordon node'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
