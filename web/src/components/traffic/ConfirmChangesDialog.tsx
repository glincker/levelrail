import { useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
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

export type ChangeKind = 'create' | 'update' | 'delete'

export interface PlannedChange {
  kind: ChangeKind
  object: string
  detail?: string
}

const KIND_ICON: Record<ChangeKind, Icon> = {
  create: PlusIcon,
  update: PencilSimpleIcon,
  delete: TrashIcon,
}

export interface ConfirmChangesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  changes: readonly PlannedChange[]
  destructive?: boolean
  typedName?: string
  confirmLabel: string
  onConfirm: () => void
  pending?: boolean
}

/** Confirmation that lists the exact objects a write will touch. */
export function ConfirmChangesDialog({
  open,
  onOpenChange,
  title,
  description,
  changes,
  destructive = false,
  typedName,
  confirmLabel,
  onConfirm,
  pending = false,
}: Readonly<ConfirmChangesDialogProps>) {
  const { t } = useTranslation('traffic')
  const [typed, setTyped] = useState('')
  const cancelRef = useRef<HTMLButtonElement>(null)
  const matches = typedName === undefined || typed === typedName
  const canConfirm = matches && !pending

  function submit(e: FormEvent) {
    e.preventDefault()
    if (canConfirm) onConfirm()
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setTyped('')
        onOpenChange(next)
      }}
    >
      <DialogContent initialFocus={cancelRef} className="sm:max-w-md">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description ? (
              <DialogDescription>{description}</DialogDescription>
            ) : null}
          </DialogHeader>
          <div className="space-y-2">
            <p className="text-sm font-medium">{t('confirm.intro')}</p>
            <ul className="space-y-1.5">
              {changes.map((change) => {
                const KindIcon = KIND_ICON[change.kind]
                return (
                  <li
                    key={`${change.kind}:${change.object}`}
                    className="flex items-start gap-2 text-sm"
                  >
                    <KindIcon
                      aria-hidden="true"
                      className={
                        change.kind === 'delete'
                          ? 'mt-0.5 size-4 shrink-0 text-destructive'
                          : 'mt-0.5 size-4 shrink-0 text-muted-foreground'
                      }
                    />
                    <span className="min-w-0">
                      <span className="sr-only">
                        {t(`confirm.kind.${change.kind}`)}
                        {': '}
                      </span>
                      <span className="font-mono text-[13px] break-all">
                        {change.object}
                      </span>
                      {change.detail ? (
                        <span className="block text-xs text-muted-foreground">
                          {change.detail}
                        </span>
                      ) : null}
                    </span>
                  </li>
                )
              })}
            </ul>
          </div>
          {typedName === undefined ? null : (
            <div className="space-y-1.5">
              <Label htmlFor="confirm-typed-name">
                {t('confirm.typeToConfirm', { name: typedName })}
              </Label>
              <Input
                id="confirm-typed-name"
                value={typed}
                autoComplete="off"
                spellCheck={false}
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>
          )}
          <DialogFooter>
            <Button
              ref={cancelRef}
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t('confirm.cancel')}
            </Button>
            <Button
              type="submit"
              variant={destructive ? 'destructive' : 'default'}
              disabled={!canConfirm}
            >
              {confirmLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
