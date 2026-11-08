import { useTranslation } from 'react-i18next'
import { FunnelGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { RemovableChip } from '@/components/ui/removable-chip'

export interface ActiveFilter {
  id: string
  label: string
  onRemove: () => void
}

/** FilterChips shows the active filters as removable chips, a clear-all and the filtered-to-empty state. */
export function FilterChips({
  filters,
  shown,
  total,
  onClearAll,
}: {
  filters: readonly ActiveFilter[]
  shown: number
  total: number
  onClearAll: () => void
}) {
  const { t } = useTranslation('uiShared')
  const active = filters.length > 0
  return (
    <>
      <p role="status" aria-live="polite" className="sr-only">
        {active ? t('filterChips.showing', { shown, total }) : ''}
      </p>
      {active ? (
        <div
          role="group"
          aria-label={t('filterChips.group')}
          className="flex min-w-0 flex-wrap items-center gap-1.5"
        >
          {filters.map((f) => (
            <RemovableChip key={f.id} label={f.label} onRemove={f.onRemove} />
          ))}
          <Button type="button" size="sm" variant="ghost" onClick={onClearAll}>
            {t('filterChips.clearAll')}
          </Button>
        </div>
      ) : null}
      {active && shown === 0 ? (
        <EmptyState
          kind="no-results"
          icon={<FunnelGlassIcon size="1.4em" />}
          title={t('filterChips.emptyTitle')}
          description={t('filterChips.emptyBody', { total })}
          action={
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onClearAll}
            >
              {t('filterChips.clearAll')}
            </Button>
          }
        />
      ) : null}
    </>
  )
}
