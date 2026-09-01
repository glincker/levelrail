import { useState } from 'react'
import { TrashIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
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
import { usePurgeAuditLog } from '../queries/auditLog'

// Manual, on-demand counterpart to the control plane's own automatic
// retention sweep (internal/api/audit_retention.go's RunAuditLogSweeper):
// for an operator who wants entries past the retention window cleared
// right now rather than waiting for the next tick. Mirrors
// DeleteNodeDialog's confirm-dialog shape: destructive, so a bare
// one-click button isn't enough, but it never deletes more than the
// server's own configured retention window already allows, so the copy
// says exactly that rather than implying a full wipe.
export function PurgeAuditLogDialog() {
  const [open, setOpen] = useState(false)
  const purgeAuditLog = usePurgeAuditLog()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      purgeAuditLog.reset()
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <TrashIcon className="size-3.5" aria-hidden="true" />
        Purge old entries
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            Purge old audit log entries?
          </DialogTitle>
          <DialogDescription>
            Deletes every audit log entry older than the configured retention
            window (APP_AUDIT_LOG_RETENTION_DAYS, 90 days by default) right
            now, instead of waiting for the next automatic sweep. This cannot
            be undone.
          </DialogDescription>
        </DialogHeader>
        {purgeAuditLog.isError ? (
          <p className="text-sm text-destructive">{purgeAuditLog.error.message}</p>
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
            disabled={purgeAuditLog.isPending}
            onClick={() => {
              purgeAuditLog.mutate(undefined, {
                onSuccess: (result) => {
                  setOpen(false)
                  toast.add({
                    title: `Purged ${result.deleted} audit log ${result.deleted === 1 ? 'entry' : 'entries'}.`,
                    type: 'success',
                  })
                },
              })
            }}
          >
            {purgeAuditLog.isPending ? 'Purging...' : 'Purge now'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
