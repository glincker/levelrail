import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { formatBytes, formatNanoCpus } from '../../../lib/format'
import type { ImpactKind } from './sizingHelpers'

export function ResourceChangeConfirm({
  open,
  impact,
  shrinks,
  memory,
  cpu,
  pending,
  onConfirm,
  onCancel,
}: {
  open: boolean
  impact: ImpactKind
  shrinks: boolean
  memory: number
  cpu: number
  pending: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation('databases')
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel()
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('sizing.confirmTitle')}</DialogTitle>
          <DialogDescription>
            {t('sizing.confirmSummary', {
              memory: memory > 0 ? formatBytes(memory) : t('sizing.noLimit'),
              cpu: cpu > 0 ? formatNanoCpus(cpu) : t('sizing.noLimit'),
            })}
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm text-foreground">
          {t(`sizing.impact.${impact}`)}
        </p>
        {shrinks ? (
          <p
            className="flex items-start gap-2 text-sm text-destructive"
            role="alert"
          >
            <TriangleAlertGlassIcon
              className="mt-0.5 size-5 shrink-0 text-tone-warning"
              aria-hidden="true"
            />
            {t('sizing.shrinkWarning')}
          </p>
        ) : null}
        <DialogFooter className="flex-wrap">
          <Button type="button" variant="outline" onClick={onCancel}>
            {t('sizing.cancel')}
          </Button>
          <Button type="button" disabled={pending} onClick={onConfirm}>
            {pending ? t('sizing.applying') : t('sizing.apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
