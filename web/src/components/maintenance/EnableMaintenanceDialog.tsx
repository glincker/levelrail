import { useTranslation } from 'react-i18next'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import type { SetDomainMaintenanceRequest } from '../../queries/domainMaintenance'
import { MaintenancePreview } from './MaintenancePreview'

// States exactly what visitors get before the site goes down, so maintenance
// is never switched on by a stray click.
export function EnableMaintenanceDialog({
  domain,
  request,
  defaultMaxSeconds,
  pending,
  error,
  onCancel,
  onConfirm,
}: {
  domain: string
  request: SetDomainMaintenanceRequest | null
  defaultMaxSeconds: number
  pending: boolean
  error?: string | null
  onCancel: () => void
  onConfirm: (req: SetDomainMaintenanceRequest | undefined) => void
}) {
  const { t } = useTranslation('maintenance')
  const req = request ?? {}
  const autoOff =
    req.until !== undefined
      ? t('confirm.endsAt', { date: new Date(req.until).toLocaleString() })
      : (req.max_duration_seconds ?? defaultMaxSeconds) > 0
        ? t('confirm.autoOff', {
            hours:
              Math.round(
                (req.max_duration_seconds ?? defaultMaxSeconds) / 360,
              ) / 10,
          })
        : t('confirm.noEnd')
  return (
    <ConfirmDialog
      open={request !== null}
      onOpenChange={(open) => {
        if (!open) onCancel()
      }}
      title={t('confirm.title', { domain })}
      description={t('confirm.description')}
      consequences={[
        t('confirm.visitors'),
        t('confirm.assets'),
        t('confirm.running'),
        t('confirm.deploys'),
        t('confirm.alerts'),
        autoOff,
      ]}
      confirmLabel={t('confirm.confirm')}
      pendingLabel={t('confirm.pending')}
      pending={pending}
      error={error}
      onConfirm={() => {
        onConfirm(request ?? undefined)
      }}
    >
      <MaintenancePreview
        title={req.title ?? ''}
        message={req.message ?? ''}
        retryAfterSeconds={req.retry_after_seconds ?? 0}
      />
    </ConfirmDialog>
  )
}
