import { useState, type ReactNode } from 'react'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'

export interface ConfirmActionProps {
  trigger: (openDialog: () => void) => ReactNode
  title: ReactNode
  description?: ReactNode
  /** Required: every destructive action must say what it will do. */
  consequences: [string, ...string[]]
  confirmLabel: string
  pendingLabel?: string
  /** Irreversible actions: the user types this exact value to enable confirm. */
  typedConfirm?: string
  pending?: boolean
  error?: string | null
  /** Call `close` once the action succeeded. */
  onConfirm: (close: () => void) => void
  /** Fires on open and close, for resetting mutation state. */
  onOpenChange?: (open: boolean) => void
}

export function ConfirmAction({
  trigger,
  title,
  description,
  consequences,
  confirmLabel,
  pendingLabel,
  typedConfirm,
  pending,
  error,
  onConfirm,
  onOpenChange,
}: ConfirmActionProps) {
  const [open, setOpen] = useState(false)

  function change(next: boolean) {
    setOpen(next)
    onOpenChange?.(next)
  }

  return (
    <>
      {trigger(() => {
        change(true)
      })}
      <ConfirmDialog
        open={open}
        onOpenChange={change}
        title={title}
        description={description}
        consequences={consequences}
        confirmLabel={confirmLabel}
        pendingLabel={pendingLabel}
        tone="destructive"
        requireTyped={typedConfirm}
        pending={pending}
        error={error}
        onConfirm={() => {
          onConfirm(() => {
            change(false)
          })
        }}
      />
    </>
  )
}
