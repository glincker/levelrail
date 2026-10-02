import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  CheckIcon,
  CopyIcon,
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
import { useRotateGitSourceWebhookSecret } from '../queries/gitSources'

// Confirm-dialog for POST /api/v1/apps/{name}/git-source/rotate-webhook-secret,
// mirroring RotateMeshKeyDialog's shape (open state reset on close,
// inline error, disabled-while-pending submit), extended with a reveal
// step: unlike a mesh key, the new value is a secret the operator must
// paste into the git provider's webhook settings, and it is never shown
// again once this dialog closes (gitSourceResource's own doc comment).
export function RotateWebhookSecretDialog({ appName }: { appName: string }) {
  const [open, setOpen] = useState(false)
  const [revealed, setRevealed] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const rotate = useRotateGitSourceWebhookSecret(appName)

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      rotate.reset()
      setRevealed(null)
      setCopied(false)
    }
  }

  function copySecret() {
    if (!revealed) return
    void navigator.clipboard.writeText(revealed).then(() => {
      setCopied(true)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={<Button type="button" variant="outline" size="sm" />}
      >
        <ArrowsClockwiseIcon className="size-3.5" aria-hidden="true" />
        Rotate secret
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {revealed ? (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <CheckIcon
                  className="size-4 text-emerald-600"
                  aria-hidden="true"
                />
                New webhook secret
              </DialogTitle>
              <DialogDescription>
                Copy this now and update the secret on the repository&apos;s
                webhook settings. It will not be shown again, and the old secret
                stopped verifying the instant this was generated.
              </DialogDescription>
            </DialogHeader>
            <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
              <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
                {revealed}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={copySecret}
              >
                {copied ? <CheckIcon /> : <CopyIcon />}
                {copied ? 'Copied' : 'Copy'}
              </Button>
            </div>
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Done
              </Button>
            </DialogFooter>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-1.5">
                <WarningIcon
                  className="size-4 text-amber-600"
                  aria-hidden="true"
                />
                Rotate webhook secret?
              </DialogTitle>
              <DialogDescription>
                Generates a fresh secret and makes it this app&apos;s live
                webhook secret immediately. The current secret stops verifying
                the moment this completes, so update the repository&apos;s
                webhook settings with the new value right away or deliveries
                will fail signature verification.
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
                  rotate.mutate(undefined, {
                    onSuccess: (resource) => {
                      if (resource.webhook_secret) {
                        setRevealed(resource.webhook_secret)
                      }
                      toast.add({
                        title: 'Webhook secret rotated.',
                        type: 'success',
                      })
                    },
                  })
                }}
              >
                {rotate.isPending ? 'Rotating...' : 'Rotate secret'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
