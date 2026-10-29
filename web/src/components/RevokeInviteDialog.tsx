import { useState } from 'react'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useRevokeInvite } from '../queries/invites'
import type { InviteResource } from '../queries/invites'

// DELETE /api/v1/invites/{id} is irreversible from this UI (there is no
// un-revoke endpoint), same reasoning RevokeTokenDialog documents for
// tokens: a confirm dialog, never a bare one-click button.
export function RevokeInviteDialog({ invite }: { invite: InviteResource }) {
  const [open, setOpen] = useState(false)
  const revokeInvite = useRevokeInvite()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revokeInvite.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        Revoke
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <TriangleAlertGlassIcon className="size-6 text-destructive" />
            Revoke invite for {invite.email}?
          </DialogTitle>
          <DialogDescription>
            The invite link stops working immediately. This cannot be undone.
          </DialogDescription>
        </DialogHeader>
        {revokeInvite.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
            <AlertDescription>{revokeInvite.error.message}</AlertDescription>
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
            disabled={revokeInvite.isPending}
            onClick={() => {
              revokeInvite.mutate(invite.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Invite for ${invite.email} revoked.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {revokeInvite.isPending ? 'Revoking...' : 'Revoke invite'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
