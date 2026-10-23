import { useTranslation } from 'react-i18next'
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
import { Skeleton } from '@/components/ui/skeleton'
import { formatMoney } from '../../lib/usageFormat'
import type { PausePreview } from '../../queries/spendCap'

/** PausePreviewDialog shows what a pause would stop and save. It never applies anything. */
export function PausePreviewDialog({
  open,
  onOpenChange,
  preview,
  pending,
  error,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  preview: PausePreview | undefined
  pending: boolean
  error: string | null
}) {
  const { t } = useTranslation('budget')
  const currency = preview?.currency || 'USD'
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('preview.title')}</DialogTitle>
          <DialogDescription>{t('preview.description')}</DialogDescription>
        </DialogHeader>
        {pending ? (
          <Skeleton className="h-20 w-full" aria-label={t('loading')} />
        ) : null}
        {error ? (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {preview ? (
          <div className="space-y-3">
            {preview.apps.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('preview.noApps')}
              </p>
            ) : (
              <ul className="divide-y divide-border rounded-lg border border-border text-sm">
                {preview.apps.map((a) => (
                  <li
                    key={a.name}
                    className="flex items-center justify-between gap-3 px-3 py-2"
                  >
                    <span className="min-w-0 truncate font-medium">
                      {a.name}
                    </span>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {t('preview.replicas', { count: a.replicas })}
                      {' · '}
                      {t('preview.saves', {
                        value: formatMoney(a.estimated_saving, currency),
                      })}
                    </span>
                  </li>
                ))}
              </ul>
            )}
            <p className="text-sm">
              {t('preview.total', {
                replicas: preview.total_replicas,
                value: formatMoney(preview.estimated_saving, currency),
                days: Math.round(preview.remaining_days),
              })}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('preview.noChanges')}
            </p>
          </div>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t('preview.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
