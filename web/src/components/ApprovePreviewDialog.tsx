import { useState } from 'react'
import { ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { useApprovePreviewEnvironment } from '../queries/previewPolicy'
import type { PreviewEnvironment } from '../types/previewEnvironment'

// ApprovePreviewDialog deploys a held fork pull request once. The dialog
// states the security implication in plain words because the fork's code
// runs with this app's environment variables and secrets.
export function ApprovePreviewDialog({
  appName,
  preview,
}: {
  appName: string
  preview: PreviewEnvironment
}) {
  const [open, setOpen] = useState(false)
  const approve = useApprovePreviewEnvironment(appName)

  function confirm() {
    approve.mutate(preview.pr_number, {
      onSuccess: () => {
        setOpen(false)
        toast.add({
          title: `Preview for PR #${preview.pr_number} is deploying.`,
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not approve preview.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" size="sm" />}>
        Approve preview
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            Approve preview for PR #{preview.pr_number}?
          </DialogTitle>
          <DialogDescription>
            This pull request comes from{' '}
            {preview.head_repo ? preview.head_repo : 'another repository'}, not
            from this app&apos;s own repository.
          </DialogDescription>
        </DialogHeader>
        <Alert variant="destructive">
          <ShieldWarningIcon aria-hidden="true" />
          <AlertDescription>
            Approving runs the fork&apos;s code on your server with this
            app&apos;s environment variables and secrets, and anyone who can
            open that page can reach them. Read the changes first. Only this
            commit deploys; a new push from the fork waits for approval again.
          </AlertDescription>
        </Alert>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={approve.isPending}
            onClick={confirm}
          >
            Approve and deploy
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
