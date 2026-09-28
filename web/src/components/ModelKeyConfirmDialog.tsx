import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { DEFAULT_GRACE_SECONDS, GRACE_PRESETS } from '../lib/modelKeyForm'
import type { ModelKey } from '../types/models'

export type KeyAction = 'rotate' | 'revoke'

// Confirms a rotate (with a grace window for the old key) or a revoke.
export function ModelKeyConfirmDialog({
  action,
  target,
  pending,
  onConfirm,
  onClose,
}: {
  action: KeyAction
  target: ModelKey
  pending: boolean
  onConfirm: (graceSeconds?: number) => void
  onClose: () => void
}) {
  const [grace, setGrace] = useState(DEFAULT_GRACE_SECONDS)
  const rotating = action === 'rotate'
  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {rotating ? 'Rotate' : 'Revoke'} key &ldquo;{target.name}&rdquo;?
          </DialogTitle>
          <DialogDescription>
            {rotating
              ? 'A new key is issued with the same name and limits. Clients must switch to it before the old key stops.'
              : 'The key stops working at once and cannot be restored. Clients using it get 401.'}
          </DialogDescription>
        </DialogHeader>
        {rotating ? (
          <div className="space-y-1.5">
            <p className="text-xs text-muted-foreground">
              Keep the old key valid for
            </p>
            <div className="flex flex-wrap gap-1.5">
              {GRACE_PRESETS.map((p) => (
                <Button
                  key={p.label}
                  type="button"
                  size="xs"
                  variant={grace === p.seconds ? 'default' : 'outline'}
                  onClick={() => {
                    setGrace(p.seconds)
                  }}
                >
                  {p.label}
                </Button>
              ))}
            </div>
          </div>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            variant={rotating ? 'default' : 'destructive'}
            disabled={pending}
            onClick={() => {
              onConfirm(rotating ? grace : undefined)
            }}
          >
            {rotating ? 'Rotate key' : 'Revoke key'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
