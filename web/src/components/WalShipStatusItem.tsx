import { useTranslation } from 'react-i18next'

import { formatDate } from '../lib/format'
import type { WalShipStatus } from '../types/pitr'

interface WalShipStatusItemProps {
  ship?: WalShipStatus
}

// One dt/dd pair for the PITR summary grid: whether the WAL archive is being
// copied to the backup target, and the last error when it is not.
export function WalShipStatusItem({ ship }: WalShipStatusItemProps) {
  const { t } = useTranslation('databases')
  const failing = Boolean(ship?.last_error)
  return (
    <div>
      <dt className="text-xs text-muted-foreground uppercase">
        {t('walShip.label')}
      </dt>
      <dd className="mt-1 text-sm text-foreground" title={t('walShip.hint')}>
        {!ship?.last_success_at && !failing ? (
          <span className="text-muted-foreground italic">
            {t('walShip.never')}
          </span>
        ) : failing ? (
          <span className="text-destructive">
            {t('walShip.failing', {
              when: formatDate(ship?.last_attempt_at, '-'),
              error: ship?.last_error,
            })}
          </span>
        ) : (
          t('walShip.ok', { when: formatDate(ship?.last_success_at, '-') })
        )}
      </dd>
    </div>
  )
}
