import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { sizeLabel } from '../images/imageFormat'
import {
  widthStep,
  type StorageCategory,
  type StorageSegment,
} from './storageBreakdown'

// Static class names so Tailwind generates them.
const SWATCH: Record<StorageCategory, string> = {
  images: 'bg-[var(--chart-series-1)]',
  containers: 'bg-[var(--chart-series-2)]',
  volumes: 'bg-[var(--chart-series-3)]',
  buildCache: 'bg-[var(--chart-series-5)]',
}

interface Props {
  segments: StorageSegment[]
  loading: boolean
  freeBytes?: number
  totalBytes?: number
}

/** StorageBreakdownBar shows size and reclaimable bytes per Docker category, in `docker system df` order. */
export function StorageBreakdownBar({
  segments,
  loading,
  freeBytes,
  totalBytes,
}: Props) {
  const { t } = useTranslation('storage')
  const sum = segments.reduce((n, s) => n + (s.totalBytes ?? 0), 0)

  return (
    <section
      aria-label={t('breakdown.label')}
      className="flex flex-col gap-3 rounded-lg border border-border p-4"
    >
      <div
        role="img"
        aria-label={t('breakdown.barLabel', { size: sizeLabel(sum) })}
        className="flex h-2.5 w-full overflow-hidden rounded-full bg-muted"
      >
        {segments.map((s) =>
          s.totalBytes === null ? null : (
            <span
              key={s.id}
              className={cn(
                'h-full border-r border-background last:border-r-0',
                SWATCH[s.id],
                widthStep(s.totalBytes, sum),
              )}
            />
          ),
        )}
      </div>
      <ul className="grid grid-cols-2 gap-x-4 gap-y-3 lg:grid-cols-4">
        {segments.map((s) => (
          <li key={s.id} className="flex min-w-0 flex-col gap-0.5">
            <span className="flex items-center gap-2 text-sm text-muted-foreground">
              <span
                aria-hidden="true"
                className={cn('size-2.5 shrink-0 rounded-full', SWATCH[s.id])}
              />
              <span className="truncate">{t(`categories.${s.id}`)}</span>
            </span>
            <span className="text-lg font-semibold tabular-nums">
              {s.totalBytes !== null
                ? sizeLabel(s.totalBytes)
                : loading
                  ? t('breakdown.measuring')
                  : t('breakdown.notMeasured')}
            </span>
            {s.totalBytes === null ? null : (
              <span
                className={cn(
                  'text-sm',
                  s.reclaimableBytes > 0
                    ? 'text-tone-warning'
                    : 'text-muted-foreground',
                )}
              >
                {s.reclaimableBytes > 0
                  ? t('breakdown.canFree', {
                      size: sizeLabel(s.reclaimableBytes),
                    })
                  : t('breakdown.nothingToFree')}
              </span>
            )}
          </li>
        ))}
      </ul>
      {freeBytes !== undefined && totalBytes !== undefined ? (
        <p className="text-sm text-muted-foreground">
          {t('breakdown.free', {
            free: sizeLabel(freeBytes),
            total: sizeLabel(totalBytes),
          })}
        </p>
      ) : null}
    </section>
  )
}
