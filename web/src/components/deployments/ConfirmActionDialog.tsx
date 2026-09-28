import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { PendingAction } from '../../hooks/useDeploymentActions'
import { confirmCopy } from '../../lib/deploymentConfirmCopy'

export interface ConfirmActionDialogProps {
  action: PendingAction | null
  busy: boolean
  onConfirm: () => void
  onDismiss: () => void
}

export function ConfirmActionDialog({
  action,
  busy,
  onConfirm,
  onDismiss,
}: ConfirmActionDialogProps) {
  const copy = action ? confirmCopy(action) : null
  return (
    <Dialog
      open={action !== null}
      onOpenChange={(o) => {
        if (!o) onDismiss()
      }}
    >
      <DialogContent>
        {copy && (
          <>
            <DialogHeader>
              <DialogTitle>{copy.title}</DialogTitle>
              <DialogDescription className="break-all">
                {copy.description}
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={onDismiss} disabled={busy}>
                Keep as is
              </Button>
              <Button
                variant={copy.danger ? 'destructive' : 'default'}
                onClick={onConfirm}
                disabled={busy}
              >
                {copy.confirm}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
