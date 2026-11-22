import { useId, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
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

/** What a destructive action takes with it, shown as plain counts before confirming. */
export interface ConfirmImpact {
  volumes?: number
  domains?: number
  /** Days existing backups are kept after the delete; omit when unknown. */
  backupRetentionDays?: number
}

/** impactParts turns an impact into the plain risk phrases, skipping zero counts. */
export function impactParts(
  impact: ConfirmImpact,
  t: (key: string, opts: Record<string, unknown>) => string,
): string[] {
  const parts: string[] = []
  if (impact.volumes) {
    parts.push(
      t('confirmDialog.impactVolumes', {
        count: impact.volumes,
        defaultValue_one: '{{count}} volume',
        defaultValue_other: '{{count}} volumes',
      }),
    )
  }
  if (impact.domains) {
    parts.push(
      t('confirmDialog.impactDomains', {
        count: impact.domains,
        defaultValue_one: '{{count}} domain',
        defaultValue_other: '{{count}} domains',
      }),
    )
  }
  if (impact.backupRetentionDays) {
    parts.push(
      t('confirmDialog.impactBackups', {
        count: impact.backupRetentionDays,
        defaultValue_one: 'backups stay {{count}} day',
        defaultValue_other: 'backups stay {{count}} days',
      }),
    )
  }
  return parts
}

export interface ConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  consequences?: string[]
  /** Counts of what is lost, rendered as one plain "You lose" line. */
  impact?: ConfirmImpact
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
  impact,
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
  const impactLine = impact ? impactParts(impact, t as never).join(', ') : ''
  const matches = requireTyped === undefined || typed === requireTyped

  function handleOpenChange(next: boolean) {
    if (!next) setTyped('')
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        initialFocus={
          focusCancel || tone === 'destructive' ? cancelRef : undefined
        }
        size="confirm"
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        {consequences && consequences.length === 1 ? (
          <p className="text-sm text-muted-foreground">{consequences[0]}</p>
        ) : null}
        {consequences && consequences.length > 1 ? (
          <ul className="list-disc space-y-1 pl-5 text-sm text-muted-foreground">
            {consequences.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        ) : null}
        {impactLine ? (
          <p className="text-sm font-medium text-destructive">
            {t('confirmDialog.impactLead', {
              items: impactLine,
              defaultValue: 'You lose: {{items}}.',
            })}
          </p>
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
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
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
