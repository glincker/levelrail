import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface EnvConflictDialogProps {
  open: boolean
  keys: string[]
  secretKeys: string[]
  saving: boolean
  onKeepMine: (keepKeys: string[]) => void
  onReloadTheirs: () => void
  onClose: () => void
}

// Shown when the env changed since it was loaded; the draft is never dropped here.
export function EnvConflictDialog({
  open,
  keys,
  secretKeys,
  saving,
  onKeepMine,
  onReloadTheirs,
  onClose,
}: EnvConflictDialogProps) {
  const { t } = useTranslation('envEditor')
  const [kept, setKept] = useState<Set<string>>(() => new Set(keys))
  const toggle = (key: string, on: boolean) => {
    setKept((prev) => {
      const next = new Set(prev)
      if (on) next.add(key)
      else next.delete(key)
      return next
    })
  }
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onClose()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('conflict.title')}</DialogTitle>
          <DialogDescription>{t('conflict.description')}</DialogDescription>
        </DialogHeader>
        <ul className="max-h-72 space-y-1.5 overflow-auto text-sm">
          {keys.map((key) => (
            <li key={key} className="flex items-center gap-2">
              <input
                id={`keep-${key}`}
                type="checkbox"
                className="size-4"
                checked={kept.has(key)}
                onChange={(e) => {
                  toggle(key, e.target.checked)
                }}
              />
              <label htmlFor={`keep-${key}`} className="font-mono break-all">
                {key}
              </label>
              <span className="text-xs text-muted-foreground">
                {secretKeys.includes(key)
                  ? t('conflict.secretChanged')
                  : t('conflict.changed')}
              </span>
            </li>
          ))}
        </ul>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onReloadTheirs}>
            {t('conflict.reload')}
          </Button>
          <Button
            type="button"
            disabled={saving}
            onClick={() => {
              onKeepMine([...kept])
            }}
          >
            {t('conflict.keepMine')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
