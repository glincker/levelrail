import { useState } from 'react'
import { ProhibitIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { useRevokeNodeCert } from '../queries/nodeCert'
import type { NodeResource } from '../types/nodeDetail'

export function RevokeNodeCertDialog({ node }: { node: NodeResource }) {
  const [open, setOpen] = useState(false)
  const revoke = useRevokeNodeCert()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revoke.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={node.cert?.state === 'revoked'}
          />
        }
      >
        <ProhibitIcon className="size-3.5" aria-hidden="true" />
        Revoke certificate
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <WarningIcon
              className="size-4 text-destructive"
              aria-hidden="true"
            />
            Revoke &ldquo;{node.name}&rdquo;&apos;s certificate?
          </DialogTitle>
          <DialogDescription>
            The control plane refuses this node&apos;s certificate from now on
            and closes its session. Workloads already running there keep running
            but can no longer be managed. Only re-enrolling the node brings it
            back.
          </DialogDescription>
        </DialogHeader>
        {revoke.isError ? (
          <p className="text-sm text-destructive">{revoke.error.message}</p>
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
            disabled={revoke.isPending}
            onClick={() => {
              revoke.mutate(node.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Certificate for "${node.name}" revoked.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {revoke.isPending ? 'Revoking...' : 'Revoke certificate'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
