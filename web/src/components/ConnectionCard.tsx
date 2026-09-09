import type { ReactNode } from 'react'
import type { Icon } from '@phosphor-icons/react'
import { CheckCircleIcon, WarningIcon, XCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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

export function ConnectionCardHeader({
  icon: HeaderIcon,
  title,
  description,
}: Readonly<{
  icon: Icon
  title: string
  description: ReactNode
}>) {
  return (
    <CardHeader>
      <div className="flex items-center gap-3">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <HeaderIcon className="size-4" />
        </div>
        <div>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
      </div>
    </CardHeader>
  )
}

export function ConfiguredStatusHeading({
  connected,
  authorized,
}: Readonly<{
  connected: boolean
  authorized: boolean
}>) {
  if (!connected) {
    return (
      <div className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
        <XCircleIcon className="size-4" />
        Not connected
      </div>
    )
  }
  return (
    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
      <CheckCircleIcon className="size-4 text-green-600 dark:text-green-400" />
      Configured
      {authorized ? (
        <Badge variant="success">authorized</Badge>
      ) : (
        <Badge variant="warning">not authorized yet</Badge>
      )}
    </div>
  )
}

export function DisconnectConnectionDialog({
  open,
  onOpenChange,
  title,
  description,
  pending,
  onConfirm,
}: Readonly<{
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: ReactNode
  pending: boolean
  onConfirm: () => void
}>) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger render={<Button variant="destructive" size="sm" />}>
        Disconnect
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5 text-destructive">
            <WarningIcon className="size-4" aria-hidden="true" />
            {title}
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" variant="destructive" disabled={pending} onClick={onConfirm}>
            {pending ? 'Disconnecting...' : 'Disconnect'}
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
    onSuccess: () => {
      toast.add({ title: successTitle, type: 'success' })
      onSuccess?.()
    },
    onError: (error: Error) => {
      toast.add({ title: errorTitle, description: error.message, type: 'error' })
    },
  }
}
