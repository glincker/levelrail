import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { sizeLabel } from '../images/imageFormat'

export type DiskHealth = 'healthy' | 'low' | 'critical'

const HEALTH_TONE: Record<DiskHealth, string> = {
  healthy: 'text-tone-success-fg',
  low: 'text-tone-warning-fg',
  critical: 'text-tone-danger-fg',
}

export interface StorageVerdictProps {
  freeBytes: number
  totalBytes: number
  health: DiskHealth
  /** Bytes the janitor stages mark safe to remove. */
  safeReclaimableBytes: number
  /** Rendered only when a number: no history means no forecast. */
  forecastDays?: number | null
  /** Primary action slot, e.g. the Review cleanup button. */
  action?: ReactNode
}

/** StorageVerdict answers "am I safe" in one sentence, with color only on the health word. */
export function StorageVerdict({
  freeBytes,
  totalBytes,
  health,
  safeReclaimableBytes,
  forecastDays,
  action,
}: StorageVerdictProps) {
  const { t } = useTranslation('storage')
  const days =
    typeof forecastDays === 'number' ? Math.round(forecastDays) : null
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-base text-foreground">
        {t('verdict.space', {
          free: sizeLabel(freeBytes),
          total: sizeLabel(totalBytes),
        })}{' '}
        <span className={cn('font-medium', HEALTH_TONE[health])}>
          {t(`verdict.health.${health}`)}
        </span>
        {'. '}
        {safeReclaimableBytes > 0
          ? t('verdict.safe', { size: sizeLabel(safeReclaimableBytes) })
          : t('verdict.nothingSafe')}
        {days !== null ? ` ${t('verdict.forecast', { count: days })}` : null}
      </p>
      {action}
    </div>
  )
}
