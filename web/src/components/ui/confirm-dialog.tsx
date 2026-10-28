import { useId, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  consequences?: string[]
  confirmLabel: string
  pendingLabel?: string
  cancelLabel?: string
  tone?: 'default' | 'destructive'
  /** The user must type this exact value before the confirm button enables. */
  requireTyped?: string
  pending?: boolean
  /** Blocks confirm for a reason the caller already explains in the body. */
  confirmDisabled?: boolean
  error?: string | null
  /** Open with Cancel focused, so a stray Enter or Space never confirms. */
  focusCancel?: boolean
  /** Solid fill for a destructive confirm instead of the pale default. */
  solidConfirm?: boolean
  onConfirm: () => void
  /** Extra controls rendered under the consequences, above the typed field. */
  children?: ReactNode
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  consequences,
  confirmLabel,
  pendingLabel,
  cancelLabel,
  tone = 'default',
  requireTyped,
  pending = false,
  confirmDisabled = false,
  error,
  focusCancel = false,
  solidConfirm = false,
  onConfirm,
  children,
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null)
  const { t } = useTranslation('common')
  const [typed, setTyped] = useState('')
  const inputId = useId()
  const matches = requireTyped === undefined || typed === requireTyped

  function handleOpenChange(next: boolean) {
    if (!next) setTyped('')
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        initialFocus={focusCancel ? cancelRef : undefined}
        className="sm:max-w-sm"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        {consequences && consequences.length > 0 ? (
          <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            {consequences.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        ) : null}
        {children}
        {requireTyped !== undefined ? (
          <div className="space-y-1.5">
            <Label htmlFor={inputId}>
              {t('confirmDialog.typePrompt', {
                defaultValue: 'Type {{value}} to confirm',
                value: requireTyped,
              })}
            </Label>
            <Input
              id={inputId}
              value={typed}
              autoComplete="off"
              spellCheck={false}
              className="font-mono"
              onChange={(event) => {
                setTyped(event.target.value)
              }}
            />
          </div>
        ) : null}
        {error ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            ref={cancelRef}
            type="button"
            variant="outline"
            className="max-sm:min-h-11"
            onClick={() => {
              handleOpenChange(false)
            }}
          >
            {cancelLabel ??
              t('confirmDialog.cancel', { defaultValue: 'Cancel' })}
          </Button>
          <Button
            type="button"
            variant={tone === 'destructive' ? 'destructive' : 'default'}
            className={cn(
              'max-sm:min-h-11',
              solidConfirm &&
                'bg-destructive text-white hover:bg-destructive/90 dark:bg-destructive dark:hover:bg-destructive/90',
            )}
            writeAction
            disabled={pending || !matches || confirmDisabled}
            onClick={onConfirm}
          >
            {pending && pendingLabel ? pendingLabel : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
