import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'

/** ConnectionActions is the separated action row of a connection card: 16px above, right-aligned, primary action last. */
export function ConnectionActions({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-2 border-t border-border pt-4">
      {children}
    </div>
  )
}

export function ConfiguredStatusHeading({
  connected,
  authorized,
}: Readonly<{
  connected: boolean
  authorized: boolean
}>) {
  const { t } = useTranslation('settings')
  if (!connected) {
    return (
      <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
        <CircleXGlassIcon className="size-4.5 text-tone-danger" />
        {t('gitConnection.notConnected')}
      </div>
    )
  }
  return (
    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
      <CircleCheckGlassIcon className="size-4.5 text-green-600 dark:text-green-400" />
      Configured
      {authorized ? (
        <Badge variant="success">authorized</Badge>
      ) : (
        <Badge variant="warning">not authorized yet</Badge>
      )}
    </div>
  )
}

// Generic consequential-action confirm dialog, not "connection"-specific
// despite the name/file: StorageAttachmentCard and DatabaseAttachmentCard
// reuse it for their own "Detach" action (same shape as a disconnect,
// same live-impact-on-a-running-app severity), rather than each hand-
// rolling a second confirm dialog.
export function DisconnectConnectionDialog({
  open,
  onOpenChange,
  title,
  description,
  pending,
  onConfirm,
  actionLabel = 'Disconnect',
  pendingLabel = 'Disconnecting...',
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  pending: boolean
  onConfirm: () => void
  actionLabel?: string
  pendingLabel?: string
}>) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        {actionLabel}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <TriangleAlertGlassIcon
              className="size-4.5 text-tone-warning"
              aria-hidden="true"
            />
            {title}
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Cancel
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={pending}
            onClick={onConfirm}
          >
            {pending ? pendingLabel : actionLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// Wraps Dialog's own onOpenChange so a form dialog always clears its
// fields on close, regardless of whether that close came from Cancel,
// Escape, or a backdrop click.
export function ResettableDialog({
  open,
  onOpenChange,
  onReset,
  children,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  onReset: () => void
  children: ReactNode
}>) {
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          onReset()
        }
        onOpenChange(next)
      }}
    >
      {children}
    </Dialog>
  )
}

export function FormDialogFooter({
  onCancel,
  submitDisabled,
  pending,
  onSubmit,
  submitLabel,
  pendingLabel,
}: Readonly<{
  onCancel: () => void
  submitDisabled: boolean
  pending: boolean
  onSubmit: () => void
  submitLabel: string
  pendingLabel: string
}>) {
  return (
    <DialogFooter>
      <Button type="button" variant="outline" onClick={onCancel}>
        Cancel
      </Button>
      <Button type="button" disabled={submitDisabled} onClick={onSubmit}>
        {pending ? pendingLabel : submitLabel}
      </Button>
    </DialogFooter>
  )
}

// Plain helper, not a component, sharing this file with the components
// above rather than a separate file for one small function.
// eslint-disable-next-line react-refresh/only-export-components
export function mutationToastCallbacks(
  successTitle: string,
  errorTitle: string,
  onSuccess?: () => void,
) {
  return {
    onSuccess: (data?: unknown) => {
      const note =
        typeof data === 'object' && data !== null && 'note' in data
          ? String(data.note)
          : undefined
      toast.add({ title: successTitle, description: note, type: 'success' })
      onSuccess?.()
    },
    onError: (error: Error) => {
      toast.add({
        title: errorTitle,
        description: error.message,
        type: 'error',
      })
    },
  }
}
