import { useTranslation } from 'react-i18next'
import type { EnvChange } from '../../lib/envRows'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface ReviewChangesDialogProps {
  open: boolean
  changes: EnvChange[]
  saving: boolean
  error: string | null
  onConfirm: () => void
  onOpenChange: (open: boolean) => void
}

const TONE = {
  added: 'success',
  changed: 'info',
  removed: 'destructive',
} as const

export function ReviewChangesDialog({
  open,
  changes,
  saving,
  error,
  onConfirm,
  onOpenChange,
}: ReviewChangesDialogProps) {
  const { t } = useTranslation('envEditor')
  const hidden = t('review.hiddenValue')
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('review.title')}</DialogTitle>
          <DialogDescription>{t('review.description')}</DialogDescription>
        </DialogHeader>
        {changes.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('review.none')}</p>
        ) : (
          <ul
            className="max-h-80 space-y-1.5 overflow-auto text-sm"
            aria-label={t('review.title')}
          >
            {changes.map((c) => (
              <li
                key={`${c.kind}-${c.key}`}
                className="flex flex-wrap items-center gap-2"
              >
                <Badge variant={TONE[c.kind]}>{t(`review.${c.kind}`)}</Badge>
                <span className="font-mono break-all">{c.key}</span>
                <span className="text-xs text-muted-foreground">
                  {t(`types.${c.type}`)}
                </span>
                <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                  {c.kind === 'added'
                    ? (c.to ?? hidden)
                    : c.kind === 'removed'
                      ? (c.from ?? hidden)
                      : `${c.from ?? hidden} → ${c.to ?? hidden}`}
                </span>
              </li>
            ))}
          </ul>
        )}
        {error ? (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('review.back')}
          </Button>
          <Button
            type="button"
            disabled={saving || changes.length === 0}
            onClick={onConfirm}
          >
            {saving ? t('review.saving') : t('review.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
