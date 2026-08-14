import { useState } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { useRevokeToken } from '../queries/tokens'
import type { TokenResource } from '../types/token'

// One row's revoke action, split out of TokenTable so that table stays
// a plain list renderer. DELETE /api/v1/auth/tokens/{id}
// (internal/api/tokens.go's handleRevokeToken) is destructive and
// irreversible (there is no un-revoke endpoint), so this is a confirm
// dialog, never a bare one-click button, per the task's explicit
// requirement and the competitor research's Dokploy-lifecycle-UX
// synthesis (finding 10: "a one-click revoke with a confirm dialog").
export function RevokeTokenDialog({ token }: { token: TokenResource }) {
  const [open, setOpen] = useState(false)
  const revokeToken = useRevokeToken()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revokeToken.reset()
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
            <WarningIcon className="size-4 text-destructive" />
            Revoke &ldquo;{token.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            Anything using this token loses access immediately. This cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        {/* Abilities in context: an admin confirming a destructive,
            irreversible action should be able to see what they're about
            to cut off without leaving this dialog to go check the table
            row again. */}
        {token.abilities.length > 0 ? (
          <p className="text-sm text-muted-foreground">
            Abilities: {token.abilities.join(', ')}
          </p>
        ) : null}
        {revokeToken.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{revokeToken.error.message}</AlertDescription>
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
            disabled={revokeToken.isPending}
            onClick={() => {
              revokeToken.mutate(token.id, {
                onSuccess: () => {
                  setOpen(false)
                  toast.add({
                    title: `Token "${token.name}" revoked.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {revokeToken.isPending ? 'Revoking...' : 'Revoke token'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
